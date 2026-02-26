# ==========================================
# Label-Only-MIA 后端服务接口 (FastAPI V3 - Batch 增强完整版)
# Member A 负责
# 功能：支持单图预测 + 批量预测 (涡轮增压模式)
# ==========================================

import torch
import torch.nn as nn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List
from contextlib import asynccontextmanager 
import uvicorn
import os

# 导入原项目的模型定义
from classifier import CNN

# ================= 配置参数 =================
MODEL_ARCH = 'CNN7'       
DATASET_NAME = 'CIFAR10' 
# 请确认这个路径是你本地真实存在的
CHECKPOINT_PATH = 'CIFAR10/target/3000/best_checkpoint_ep.pth' 
DEVICE = 'cuda' if torch.cuda.is_available() else 'cpu'

# 图像规格常量 (必须与 Go 端的 constants 保持一致)
IMG_CHANNELS = 3
IMG_HEIGHT = 32
IMG_WIDTH = 32
FLATTENED_SIZE = IMG_CHANNELS * IMG_HEIGHT * IMG_WIDTH # 3072
# ===========================================

# 全局模型变量
model = None

# --- 生命周期管理 (Lifespan) ---
@asynccontextmanager
async def lifespan(app: FastAPI):
    global model
    print(f"\n======== 服务启动初始化 (Lifespan V3 + Batch) ========")
    print(f"[*] 检测设备: {DEVICE}")
    print(f"[*] 模型路径: {CHECKPOINT_PATH}")
    
    # 1. 检查文件是否存在
    if not os.path.exists(CHECKPOINT_PATH):
        print(f"[!] 严重错误：找不到模型文件 {CHECKPOINT_PATH}")
        print("请先运行 main.py (action=0) 训练模型！")
    else:
        # 2. 初始化模型骨架
        print("[*] 初始化 CNN7 结构...")
        model_instance = CNN(MODEL_ARCH, DATASET_NAME)
        
        # 3. 加载权重
        try:
            print("[*] 正在加载 .pth 文件...")
            # weights_only=False 兼容旧版 pytorch 格式
            checkpoint = torch.load(CHECKPOINT_PATH, map_location=DEVICE, weights_only=False)
            
            # 智能拆包逻辑
            state_dict_to_load = None
            if isinstance(checkpoint, dict) and 'state_dict' in checkpoint:
                print("[*] 检测到模型是 Checkpoint 包裹格式，正在提取 state_dict...")
                state_dict_to_load = checkpoint['state_dict']
            else:
                print("[*] 未发现包裹，尝试直接加载权重...")
                state_dict_to_load = checkpoint

            # 去除 DataParallel 前缀
            clean_state_dict = {}
            for k, v in state_dict_to_load.items():
                if k.startswith('module.'):
                    clean_state_dict[k[7:]] = v 
                else:
                    clean_state_dict[k] = v
            
            # 装载
            model_instance.load_state_dict(clean_state_dict)
            
            # 送入设备
            model_instance.to(DEVICE)
            model_instance.eval() 
            model = model_instance
            print("[+] 权重加载成功！Success！")
            
        except Exception as e:
            print(f"\n[!!!] 模型加载崩溃 [!!!]")
            print(f"错误信息: {str(e)}")
            raise e

    print(f"[+] 服务就绪！")
    print(f"    - 单图接口: POST /predict")
    print(f"    - 批量接口: POST /predict_batch (Turbo Mode)")
    print("==============================================\n")
    
    yield # 服务运行中
    
    print("\n[*] 服务正在关闭，清理显存...")
    model = None
    if torch.cuda.is_available():
        torch.cuda.empty_cache()
    print("[*] Bye Bye!")

app = FastAPI(title="MIA Attack Oracle", lifespan=lifespan)

# ==========================================
# 数据结构定义 (Protocol)
# ==========================================

# 单图请求/响应
class PredictRequest(BaseModel):
    image: List[float] # [3072]

class PredictResponse(BaseModel):
    label: int           
    logits: List[float]  

# 批量请求/响应 (支持 Go 端 PredictBatch)
class BatchPredictRequest(BaseModel):
    # 这是一个二维数组 [[3072], [3072], ...]
    images: List[List[float]] 

class BatchPredictResponse(BaseModel):
    labels: List[int]            # 预测标签列表
    logits_batch: List[List[float]] # 对应的 logits 列表 (用于复杂攻击)

# ==========================================
# 接口实现
# ==========================================

# 1. 单图预测接口 (保留以兼容旧代码)
@app.post("/predict", response_model=PredictResponse)
async def predict(req: PredictRequest):
    if model is None:
        raise HTTPException(status_code=500, detail="Model not loaded")
    
    if len(req.image) != FLATTENED_SIZE:
        raise HTTPException(status_code=400, detail=f"Shape error. Expected {FLATTENED_SIZE}")

    try:
        # [3072] -> [1, 3, 32, 32]
        input_tensor = torch.tensor(req.image).float().to(DEVICE)
        input_tensor = input_tensor.view(1, IMG_CHANNELS, IMG_HEIGHT, IMG_WIDTH)
        
        with torch.no_grad():
            output = model(input_tensor)
            pred_label = output.argmax(dim=1).item()
            logits_list = output.cpu().squeeze().tolist()
            return PredictResponse(label=pred_label, logits=logits_list)
            
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

# 2. 批量预测接口 (Turbo Mode / 涡轮增压)
# 这个接口专门为了对接 Member B 的 PredictBatch 方法
@app.post("/predict_batch", response_model=BatchPredictResponse)
async def predict_batch(req: BatchPredictRequest):
    if model is None:
        raise HTTPException(status_code=500, detail="Model not loaded")
    
    batch_size = len(req.images)
    if batch_size == 0:
        return BatchPredictResponse(labels=[], logits_batch=[])

    # 简单的第一张图形状检查 (为了速度不检查全部，相信 Go 端的预处理)
    if len(req.images[0]) != FLATTENED_SIZE:
        raise HTTPException(status_code=400, detail=f"Image shape error. Expected {FLATTENED_SIZE}")

    try:
        # 数据转换：List[List] -> Tensor [N, 3072] -> GPU
        input_tensor = torch.tensor(req.images).float().to(DEVICE)
        
        # 核心：动态 Reshape [N, 3072] -> [N, 3, 32, 32]
        # view 的第一个参数 -1 让 pytorch 自动计算 Batch Size
        input_tensor = input_tensor.view(-1, IMG_CHANNELS, IMG_HEIGHT, IMG_WIDTH)
        
        with torch.no_grad():
            # 批量推理 (RTX 4060 最擅长这个)
            output = model(input_tensor) # shape: [N, 10] (假设10分类)
            
            # 获取所有标签: shape [N]
            pred_labels = output.argmax(dim=1).cpu().tolist()
            
            # 获取所有 Logits: shape [N, 10] -> List[List]
            pred_logits = output.cpu().tolist()
            
            return BatchPredictResponse(labels=pred_labels, logits_batch=pred_logits)
            
    except Exception as e:
        # 打印错误方便调试
        print(f"[Batch Error] {str(e)}")
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    uvicorn.run("server:app", host="0.0.0.0", port=8080, reload=False)
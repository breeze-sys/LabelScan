# ==========================================
# Label-Only-MIA 后端服务接口 (FastAPI Lifespan 最新标准版)
# Member A 负责
# ==========================================

import torch
import torch.nn as nn
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from typing import List
from contextlib import asynccontextmanager # 引入这个新工具来管理生命周期
import uvicorn
import os

# 导入原项目的模型定义
from classifier import CNN

# ================= 配置参数 =================
MODEL_ARCH = 'CNN7'       
DATASET_NAME = 'CIFAR10' 
# 请确认这个路径是你本地真实存在的
CHECKPOINT_PATH = 'results/CIFAR10/target/3000/best_checkpoint_ep.pth' 
DEVICE = 'cuda' if torch.cuda.is_available() else 'cpu'
# ===========================================

# 全局模型变量
model = None

# --- 【核心升级】新版生命周期管理 (Lifespan) ---
# 这一整块代码替代了旧版的 @app.on_event("startup")
# 逻辑：yield 之前的部分是【启动时】运行，yield 之后的部分是【关闭时】运行
@asynccontextmanager
async def lifespan(app: FastAPI):
    global model
    print(f"\n======== 服务启动初始化 (Lifespan Mode) ========")
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
        
        # 3. 加载权重 (保留核心修复逻辑)
        try:
            print("[*] 正在加载 .pth 文件...")
            # weights_only=False 兼容旧版 pytorch 格式
            checkpoint = torch.load(CHECKPOINT_PATH, map_location=DEVICE, weights_only=False)
            
            # 智能拆包逻辑：判断是直接权重还是包含 meta 信息的字典
            state_dict_to_load = None
            if isinstance(checkpoint, dict) and 'state_dict' in checkpoint:
                print("[*] 检测到模型是 Checkpoint 包裹格式，正在提取 state_dict...")
                state_dict_to_load = checkpoint['state_dict']
            else:
                print("[*] 未发现包裹，尝试直接加载权重...")
                state_dict_to_load = checkpoint

            # 去除 DataParallel 留下的 'module.' 前缀
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
            # 这里即使报错，我们也抛出，方便在终端看到具体原因
            raise e

    print(f"[+] 服务就绪！监听接口: POST /predict")
    print("==============================================\n")
    
    yield # --- 服务器启动成功，在这里挂起，等待 Go 发请求 ---
    
    # --- 下面是关闭服务时自动运行的清理逻辑 (优雅退出) ---
    print("\n[*] 服务正在关闭，清理显存...")
    model = None
    if torch.cuda.is_available():
        torch.cuda.empty_cache()
    print("[*] Bye Bye!")

# 关键点：在这里把 lifespan 注册进去
app = FastAPI(title="MIA Attack Oracle", lifespan=lifespan)

# --- 数据接口定义 ---
class PredictRequest(BaseModel):
    # Go 传过来的是展平的数组，3072个浮点数
    image: List[float] 

class PredictResponse(BaseModel):
    label: int           
    logits: List[float]  

# --- 预测接口 ---
@app.post("/predict", response_model=PredictResponse)
async def predict(req: PredictRequest):
    if model is None:
        raise HTTPException(status_code=500, detail="Model is not loaded properly")
    
    # 尺寸校验 CIFAR10: 3通道 * 32宽 * 32高 = 3072
    if len(req.image) != 3072:
        raise HTTPException(status_code=400, detail=f"Size mismatch. Expected 3072, got {len(req.image)}")

    try:
        # List -> Tensor -> GPU
        input_tensor = torch.tensor(req.image).float().to(DEVICE)
        input_tensor = input_tensor.view(1, 3, 32, 32) # Reshape
        
        with torch.no_grad():
            output = model(input_tensor)
            # 获取预测类别 (Label)
            pred_label = output.argmax(dim=1).item()
            # 获取原始分数 (Logits) - 传给 Go 算 Loss
            logits_list = output.cpu().squeeze().tolist()
            
            return PredictResponse(label=pred_label, logits=logits_list)
            
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))

if __name__ == "__main__":
    # 启动命令
    uvicorn.run("server:app", host="0.0.0.0", port=8080, reload=False)
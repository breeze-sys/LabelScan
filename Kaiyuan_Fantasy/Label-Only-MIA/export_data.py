# export_data.py
import torch
import numpy as np
from utils import load_dataset
import os
import struct

# 配置：跟 main.py 里的保持一致
args = type('Args', (), {})()
args.batch_size = 1
args.dataset_ID = 0 
args.datasets = ['CIFAR10']
# 我们需要导出用来训练的那 3000 张 (Member)
# 以及测试用的 (Non-Member)
target_train_size = 3000 

print(f"[*] 正在加载数据集 (TrainSize={target_train_size})...")
# 复用 utils.py 的逻辑，保证顺序一模一样
# mode='target' 会返回 train_loader (Member) 和 test_loader (Non-Member)
train_loader, test_loader = load_dataset(args, 'CIFAR10', target_train_size, mode='target')

# 定义导出路径（直接导到你的 Go 项目里去）
# 请确保这个路径也是你 Go 项目的路径
GO_PROJECT_PATH = "../Go-MIA-Attack/data" 
os.makedirs(GO_PROJECT_PATH, exist_ok=True)

def save_to_binary(loader, filename_prefix, max_count=100):
    """
    把图片和标签存成纯二进制文件：
    格式：[Label (1 int32)] [Pixels (3072 float32)] ... 循环
    """
    file_path = os.path.join(GO_PROJECT_PATH, f"{filename_prefix}.bin")
    print(f"[*] 正在写入 {file_path} ...")
    
    count = 0
    with open(file_path, 'wb') as f:
        for batch_idx, (data, target) in enumerate(loader):
            if count >= max_count: break
            
            # data: Tensor [1, 3, 32, 32] -> numpy flat array
            # 这里不用除以255，因为 pytorch loader 出来已经是 0-1 的 float 了
            img_np = data.numpy().flatten().astype(np.float32)
            label = int(target.item())
            
            # 1. 写入 Label (int32, 4字节)
            f.write(struct.pack('<i', label)) # 小端序
            
            # 2. 写入 Pixels (3072 * 4字节)
            f.write(img_np.tobytes())
            
            count += 1
            if count % 100 == 0:
                print(f"    已导出 {count} 张...")
    
    print(f"[+] {filename_prefix} 完成！共 {count} 张。")

# 为了测试快一点，我们先只导出 50 张 Member 和 50 张 Non-Member
# 等流程跑通了，再把 max_count 改大
save_to_binary(train_loader, "members", max_count=50)
save_to_binary(test_loader, "non_members", max_count=50)
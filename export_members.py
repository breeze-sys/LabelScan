# export_members.py
import numpy as np
import json

# CIFAR-10 的总训练集规模是 50,000
train_size = 3000
remain_size = 46000
test_size = 1000
lengths = [train_size, remain_size, test_size]

# 1. 严格还原 utils.py 里的索引数组生成
indices = list(range(sum(lengths)))

# 2. 严格还原 utils.py 里的洗牌种子
np.random.seed(1)
np.random.shuffle(indices)

# 3. 目标模型的训练集是洗牌后的前 3000 个（对应 indices[0:3000]）
#    我们的 Go 测试只需要取其中的前 50 个真成员
true_members = indices[:50]

# 4. 导出为 Go 能够直接读取的 target_members.json 文件
with open("target_members.json", "w") as f:
    json.dump(true_members, f)

print("🎉 成功！50个真实成员的物理索引已写入 target_members.json")
print("📊 前 5 个真实的成员物理索引为:", true_members[:5])
# Label-Only-MIA-Go

一个基于 Go 和 Python 的黑盒成员推断审计原型项目。

目前仓库的整体思路是：

- Go 侧负责审计流程编排、攻击逻辑、并发处理和结果输出
- Python 侧负责加载模型并提供 HTTP 推理接口
- 数据集当前主要围绕 CIFAR-10

## 项目功能

这个项目用于对目标模型做 label-only membership inference audit，大体流程包括：

- 加载目标模型与影子模型相关配置
- 读取样本数据
- 调用模型接口获取预测结果
- 结合影子模型信号和边界攻击结果做成员风险判断
- 输出审计报告

## 主要文件

- `main.go`
  - 项目主入口，串起完整审计流程
- `pkg/attack/`
  - 攻击相关实现，当前包含 HSJA 相关逻辑
- `pkg/audit/`
  - 审计核心逻辑与最终判定
- `pkg/client/`
  - Go 侧 HTTP 客户端，用来请求 Python 模型服务
- `pkg/dataset/`
  - 数据读取逻辑
- `pkg/worker/`
  - 并发任务处理
- `pkg/mathutils/`
  - 数学与统计辅助函数
- `python_server/server.py`
  - Python 模型服务入口
- `python_server/classifier.py`
  - Python 侧模型定义
- `python_server/calc_thresholds.py`
  - 阈值计算脚本
- `shadow_config.json`
  - 审计阈值配置
- `output/audit_report.json`
  - 审计结果输出示例

## 运行说明

当前默认需要先启动两个 Python 服务：

- 目标模型服务
- 影子模型服务

然后在项目根目录运行 Go 主程序：

```bash
go run .
```

如需重新生成阈值配置，可在 `python_server/` 下运行：

```bash
python calc_thresholds.py
```

## 说明

这个 README 只保留简要说明，便于后续继续调整项目结构和实现细节。

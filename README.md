# LabelScan-Go

面向分类模型的 Label-Only 成员推理风险审计工具，融合边界搜索、影子模型信号与 Docker 化复现。

LabelScan-Go 是一个面向分类模型的 label-only membership inference audit 工具。目标模型侧只依赖预测标签；影子模型侧使用必要的分类分数接口来计算 loss，从而评估某批样本是否可能出现在目标模型的训练集中，并输出可读的成员风险报告。

本项目由 Go 审计引擎和 Python 模型服务组成。Go 侧负责并发调度、边界搜索、信号融合和报告生成；Python 侧负责加载目标模型与影子模型，并以 HTTP API 的形式提供推理能力。当前仓库内置 CIFAR-10 示例模型，便于直接复现完整流程。

## 快速运行

推荐使用 Docker Compose 在本地启动完整服务：

```bash
docker compose up --build
```

启动完成后打开：

```text
http://localhost:8080
```

页面中可以先点击“检查服务状态”，确认 `target-oracle` 与 `shadow-oracle` 均为 `ok`，再运行 `smoke` 模式完成一次快速审计。审计模式默认是 `full`，会同时使用边界攻击和影子模型；如果只想接入一个外部目标 API，可以切换到 `boundary-only`，此时不要求 Shadow 服务在线。

报告输出的是“高/较高/中等/低成员风险”，不是对训练集归属的确定性断言。目标模型准确率、数据分布和查询预算都会影响结果，建议结合报告中的 shadow loss、边界距离和波动系数进行复核。

审计结束后会生成：

```text
output/web_audit_report.json
output/web_audit_report.html
```

如果只想运行命令行版本：

```bash
docker compose --profile cli run --rm audit-runner
```

## 本项目做什么

本项目关注的问题是：在只能访问模型预测接口的情况下，能否判断某个样本是否曾参与目标模型训练。

训练集成员通常会在目标模型上表现出更稳定、更自信的决策行为。LabelScan-Go 将这种行为拆成两类信号：

- 行为信号：通过影子模型估计样本的成员化倾向，重点观察 loss、置信分布和阈值位置。
- 几何信号：通过边界攻击估计样本到目标模型决策边界的距离，并观察轻微扰动下的稳定性。
- 融合判断：当行为信号和几何信号同时支持成员推断时，样本会被判为高风险；单一证据不足时会降级为疑似或安全。

这种设计避免只看单个 loss 阈值造成的误判，也能让输出结果更接近实际审计场景中的“证据链”。

## 审计模式

`full` 是默认模式，包含边界攻击、影子模型 loss 和融合判断，适合当前仓库内置模型或已经完成 shadow 校准的外部任务。

`boundary-only` 只调用目标模型的 label-only 接口，直接复用 HopSkipJump 边界搜索。这个模式不需要用户先训练影子模型，适合作为外部 API 的快速接入和初筛；代价是证据更弱，报告中的风险判断主要来自边界距离和局部稳定性。因此它应被解释为风险提示，而不是成员归属结论。

## 为什么是 Label-Only

许多真实服务不会开放训练过程、梯度或中间层特征，用户通常只能拿到预测结果。Label-only MIA 的目标就是在这种受限条件下开展审计。

目标模型侧只依赖预测标签和批量标签接口；影子模型侧会使用 `/predict_logits` 计算 loss，这是为了让本地复现和报告更稳定。对于只有标签输出的外部服务，可以先将目标服务作为 `Target API` 接入，再使用本地影子模型和阈值配置完成审计。

## 核心方法

### 影子模型信号

影子模型用于模拟目标模型对成员样本的响应轮廓。我们不会追求影子模型越强越好，而是希望它能稳定刻画“目标模型可能记住过的样本”在 loss 分布上的位置。`shadow_config.json` 保存了当前影子模型的阈值配置，Go 审计引擎会读取其中的 `threshold` 和 `mean_member_loss`，并根据 loss 越小越像成员的原则做风险判断。

### 边界攻击信号

本项目使用 HopSkipJump Attack 思路估计样本到目标模型决策边界的距离。成员样本往往处在模型更熟悉、更稳定的区域；非成员样本在轻微扰动后更容易表现出不稳定的边界行为。Go 侧会对原图和若干微扰版本并发执行边界搜索，得到平均边界距离和波动系数。

### 信号融合

单一信号容易受到模型精度、样本难度和数据分布偏移影响。因此本项目将影子模型 loss 与边界几何信号合并判断：

- 两类信号都强时，输出高风险成员判断。
- 只有一类信号强时，输出疑似风险。
- 目标模型原图预测错误或证据不足时，输出低成员风险。

最终报告会同时保留每个样本的真实成员标记、预测风险、shadow loss、边界距离和波动系数，便于后续复核。

## 为什么使用 Go

审计流程中包含大量独立样本、独立扰动点和重复 HTTP 请求。Go 的 goroutine 和 worker pool 很适合这类并发任务，代码结构也更容易把攻击流程、模型客户端、数据读取和报告输出拆开维护。

Python 仍然用于模型加载和推理，因为 PyTorch 生态更成熟。两部分通过 HTTP 协议衔接，既保留了 Python 训练/推理生态，也让审计引擎更容易扩展到其他模型服务。

## 外部模型接入

Web 控制台中的 `Target API` 和 `Shadow API` 可以替换为外部兼容服务地址。统一实现以下接口最方便：

```text
POST /predict
POST /predict_logits
POST /predict_batch
GET  /health
```

如果拆分实现，Target 服务至少需要 `POST /predict`、`POST /predict_batch` 和 `GET /health`；Shadow 服务需要 `POST /predict_logits` 和 `GET /health`。

请求体示例：

```json
{
  "image": [0.0, 0.1, 0.2]
}
```

实际图像向量需要与模型输入一致；当前 CIFAR-10 示例使用长度为 `3072` 的展平向量。

返回体示例：

```json
{
  "label": 3,
  "logits": [0.1, 0.2, 1.8]
}
```

需要注意的是，任意外部 AI 并不能直接“一键检测”。实际接入时仍需要准备与目标任务一致的数据格式、影子模型和阈值配置。当前版本提供的是可运行的兼容接口和本地复现路径。

完整接入外部模型通常分三步：

1. 先把外部目标模型包成兼容的 `Target API`，用 `boundary-only` 验证数据格式、标签空间和调用稳定性。
2. 准备与目标任务同分布的数据，调用目标 API 生成标签或伪标签，形成 shadow 训练数据。
3. 在本地训练 shadow model，并重新生成 `shadow_config.json`，再切换到 `full` 模式运行完整审计。

## 常用命令

本地 Go 运行：

```bash
go run . --serve
```

本地命令行审计：

```bash
go run . --preset smoke
```

指定 API 地址：

```bash
go run . --serve \
  --audit-mode full \
  --target-api http://localhost:8000 \
  --shadow-api http://localhost:8001
```

重新生成影子模型阈值：

```bash
cd python_server
python calc_thresholds.py
```

## 目录结构

```text
.
├── main.go                         # CLI 与 Web 控制台入口
├── docker-compose.yml              # 本地完整服务编排
├── docker/                         # Go 与 Python 镜像构建文件
├── pkg/attack/                     # 边界攻击实现
├── pkg/audit/                      # 风险信号与融合逻辑
├── pkg/client/                     # HTTP 模型客户端
├── pkg/dataset/                    # CIFAR-10 数据读取
├── pkg/mathutils/                  # loss、距离、统计工具
├── pkg/worker/                     # 并发审计任务池
├── python_server/server.py         # PyTorch 模型推理服务
├── python_server/classifier.py     # CNN 模型定义
├── python_server/calc_thresholds.py# 影子模型阈值计算
├── scripts/evaluation/             # 可选评估与成员索引导出脚本
├── shadow_config.json              # 当前影子模型阈值
└── target_members.json             # 当前目标模型成员索引
```

## 维护说明

- 更换目标模型时，需要同步更新目标模型权重、成员索引和相应的数据读取逻辑。
- 更换影子模型时，需要重新生成 `shadow_config.json`，否则 loss 阈值与模型不匹配。
- 完整审计会执行大量边界搜索，耗时明显高于普通推理；日常自检建议先使用 `smoke` 模式。
- Docker 默认安装 CPU 版 PyTorch，优先保证复现稳定性。需要 GPU 推理时，可以直接在宿主机运行 `python_server/server.py`，或另行构建 CUDA 版镜像。

## License

本项目源码采用 MIT License 发布，详见 [LICENSE](LICENSE)。

第三方依赖、容器基础镜像、数据集与模型资产的许可说明见
[THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。CIFAR-10 等第三方数据资产
不由本项目许可证重新授权，使用者应遵循其原始发布条款。

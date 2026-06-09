# Label-Only-MIA-Go

面向分类模型的 Label-Only 成员推理风险审计工具，融合边界搜索、影子模型信号与 Docker 化本地部署。系统接入图像分类模型预测接口，在无需目标模型梯度、参数或概率分数的条件下，批量评估样本训练集成员风险并生成可复核审计报告。

命名说明：本仓库和正式项目名称统一为 **Label-Only-MIA-Go**；Web 控制台、HTML 报告和运行日志中的 **LabelScan-Go** 是本项目的工具展示名称，用于指代本地审计控制台与报告系统。

Label-Only-MIA-Go 的理论基础来源于 ACM CCS 2021 论文 *Membership Leakage in Label-Only Exposures*。项目聚焦成员推理攻击（Membership Inference Attack, MIA）：在只能访问模型预测标签、无法获取梯度、参数或概率分数的条件下，判断某个样本是否可能参与过目标模型训练。相比只保留论文实验流程的原型代码，本项目将边界攻击、影子模型信号、并发调度、RESTful 接入和 Docker 部署整理为一个可本地复现、可扩展接入外部模型的审计工具。

## 项目概览

真实模型服务通常不会暴露训练过程、梯度或中间层特征，很多场景下用户只能获得分类标签。传统依赖置信度或概率分数的成员推理方法在这种条件下难以使用，因此本项目采用两类互补信号完成审计：

- 迁移攻击信号：通过影子模型模拟目标模型行为，并利用样本在影子模型上的交叉熵损失（loss）判断模型是否表现出“记忆”特征。
- 边界攻击信号：通过 HopSkipJump Attack 思路向样本添加微小扰动，估计样本到目标模型决策边界的距离和扰动稳定性。
- 融合判定：将 shadow loss、边界距离和局部波动系数合并为样本级风险等级，减少单一指标造成的误判。

系统最终输出的是“成员风险等级”和对应证据指标，而不是对训练集归属的确定性断言。目标模型准确率、数据分布、查询预算和影子模型质量都会影响结果，报告应作为安全审计和人工复核的依据。

## 系统架构

项目采用 Go + Python 的解耦式架构：

- Go 审计引擎负责样本调度、并发查询、边界搜索、风险融合和报告生成。
- Python 模型服务负责加载 PyTorch 目标模型与影子模型，并通过 FastAPI 提供推理接口。
- Docker Compose 负责在本地启动完整服务，默认包含目标模型服务、影子模型服务和 Web 控制台。

选择 Go 重写核心审计流程，主要是因为边界搜索和批量审计包含大量独立 HTTP 请求。Go 的 goroutine 和 worker pool 能较自然地处理高并发网络 I/O，也更适合把攻击流程、模型客户端、数据读取和报告输出拆成可维护的工程模块。Python 继续承担模型加载和训练相关工作，以保留 PyTorch 生态的便利性。

## 服务器部署与 Windows 启动器

如果要做“后端在服务器运行、用户只拿 Windows exe”的交付方式，推荐把本仓库部署为服务器端 Docker 服务，再让 exe 只打开受保护的 HTTPS 控制台。

### 服务器端

生产环境建议使用后台启动：

```bash
docker compose up -d --build
```

当前 Compose 只把 Web 控制台映射到宿主机 `127.0.0.1:8080`，Target Oracle `8000` 和 Shadow Oracle `8001` 只在 Docker 内网暴露，避免模型推理服务直接进入公网。若只是临时用服务器 IP 调试 Web 页面，可以显式放开 Web 绑定：

```bash
HOST_WEB_BIND=0.0.0.0 docker compose up -d --build
```

正式上线建议仍保持只监听 `127.0.0.1`，再由 Cloudflare Tunnel 或 Nginx 对外提供 HTTPS。Nginx 示例配置见 `deploy/nginx/labelscan.conf.example`，包含 HTTPS 反向代理、Basic Auth 和 `/api/audit` 限流。若改用该 Nginx 模板，部署时把模板里的 `labelscan.your-domain.com` 替换为 `labelscan.site` 或你的真实域名，并创建 Basic Auth 用户文件：

```bash
sudo htpasswd -c /etc/nginx/.htpasswd-labelscan labelscan
```

### Windows 启动器

最小可交付 exe 是纯 Go 启动器，不内置 Docker、Python、模型权重或审计逻辑，只负责打开服务器上的 Web 控制台。构建命令：

```bash
./scripts/build_windows_launcher.sh https://labelscan.site
```

生成文件：

```text
dist/LabelScan-Go.exe
dist/labelscan.url
```

`labelscan.url` 放在 exe 同目录时会覆盖编译时默认地址，所以更换域名时可以直接改这个文件。若已经确定长期域名，也可以把域名作为脚本第一个参数重新构建 exe。

### 无 sudo / 免 Docker 本地运行

如果服务器账号没有 sudo 权限，也没有 Docker daemon 访问权限，可以使用 Conda 环境直接运行三段服务。当前服务器是 Blackwell GPU，推荐安装 PyTorch CUDA 13.0 wheel：

```bash
conda create -y -n labelscan --override-channels -c https://mirrors.tuna.tsinghua.edu.cn/anaconda/cloud/conda-forge python=3.11 pip go
conda run -n labelscan python -m pip install -i https://pypi.tuna.tsinghua.edu.cn/simple fastapi==0.115.6 'uvicorn[standard]==0.34.0' numpy==1.26.4
conda run -n labelscan python -m pip install --index-url https://download.pytorch.org/whl/cu130 --extra-index-url https://pypi.tuna.tsinghua.edu.cn/simple torch==2.12.0+cu130 torchvision==0.27.0+cu130
conda run -n labelscan python -m pip install -i https://pypi.tuna.tsinghua.edu.cn/simple numpy==1.26.4
```

启动本地服务：

```bash
./scripts/start_local_no_docker.sh
```

默认使用物理 GPU 0 运行 Target Oracle 和 Shadow Oracle，适合共享服务器上避开其他用户任务。需要拆到两张空闲 GPU 时可以覆盖环境变量：

```bash
TARGET_CUDA_VISIBLE_DEVICES=0 SHADOW_CUDA_VISIBLE_DEVICES=1 ./scripts/start_local_no_docker.sh
```

默认端口只监听服务器本机，不对公网开放：

```text
Target Oracle: http://127.0.0.1:18000
Shadow Oracle: http://127.0.0.1:18001
Web console: http://127.0.0.1:18080
```

Windows 端使用 SSH 隧道访问：

```bash
ssh -N -L 18080:127.0.0.1:18080 <user>@<server>
```

然后打开：

```text
http://127.0.0.1:18080
```

停止服务：

```bash
./scripts/stop_local_no_docker.sh
```

如果同时使用 Cloudflare Tunnel 或需要自动拉起，建议使用统一服务脚本：

```bash
# 启动后端和 Cloudflare Tunnel
./scripts/labelscan_service.sh start

# 启动 watchdog，默认每 60 秒检查一次，掉线会自动拉起
./scripts/labelscan_service.sh start-watchdog

# 查看后端、Tunnel 和 watchdog 状态
./scripts/labelscan_service.sh status

# 手动关闭全部服务。若 watchdog 正在运行，必须用这个命令关闭
./scripts/labelscan_service.sh stop

# 手动重启全部服务
./scripts/labelscan_service.sh restart
```

watchdog 日志默认写入 `output/labelscan-watchdog.log`。服务器重启后的自动恢复可以使用用户级 `crontab @reboot` 调用 `./scripts/labelscan_service.sh start-watchdog`，不需要 sudo。

Web 控制台的审计任务采用队列模式：同时只运行一个任务，其余任务排队等待。页面中的“取消当前任务”按钮可以取消自己提交的排队任务；若任务正在运行，后端会向审计流程发送取消信号，并在当前模型请求返回后尽快停止。

## 快速开始

推荐使用 Docker 运行。Windows、macOS 和 Linux 均可使用 Docker Desktop 或 Docker Engine；Windows 用户建议开启 WSL2 后端。

1. 下载完整项目到本地：

```bash
git clone <repository-url>
cd Label-Only-MIA-Go
```

2. 在项目根目录启动完整服务：

```bash
docker compose up --build
```

需要在包含 `docker-compose.yml` 的项目根目录执行命令。项目可以放在任意本地路径，但仓库内部目录结构应保持不变，否则模型权重、数据、配置和报告输出路径可能无法正确匹配。

3. 打开 Web 控制台：

```text
http://localhost:8080
```

页面加载后，可以先点击“检查服务状态”，确认 Target Oracle 和 Shadow Oracle 均可访问；随后选择审计模式和样本规模，点击“运行风险评估”即可生成结果。

审计完成后会生成：

```text
output/web_audit_report.json
output/web_audit_report.html
```

Web 页面会展示样本级风险、准确率、精确率、召回率、高风险比例等汇总指标；HTML 报告保留 shadow loss、边界距离、波动系数和每个样本的判定证据，便于后续复核。

## Web 控制台使用

### 审计模式

`full` 是默认模式，会同时使用边界攻击信号和影子模型 loss 信号，适合仓库内置 CIFAR-10 示例模型，或已经完成影子模型训练与阈值校准的外部任务。

`boundary-only` 只调用目标模型的 label-only 接口，不要求 Shadow 服务在线。该模式适合接入外部 API 时做快速初筛，但证据弱于 `full` 模式，输出结果应理解为边界行为风险提示。

### 样本规模

Web 控制台中的样本规模对应一组运行参数：

| 预设 | 用途 | 成员/非成员样本 | 校准规模 | 查询预算 |
| --- | --- | --- | --- | --- |
| `smoke` | 快速自检，确认服务、路径和报告生成流程是否正常 | 1 / 1 | 10 个候选样本，取 1 个有效路人样本 | 最大 800 次查询，8 轮边界迭代 |
| `standard` | 常规审计，兼顾速度和结果稳定性 | 50 / 50 | 100 个候选样本，取 10 个有效路人样本 | 最大 5000 次查询，40 轮边界迭代 |
| `extended` | 扩展审计，使用更多样本和更充分的现场校准 | 100 / 100 | 200 个候选样本，取 20 个有效路人样本 | 最大 5000 次查询，40 轮边界迭代 |
| `custom` | 自定义运行参数 | 以页面或命令行输入为准 | 以页面或命令行输入为准 | 以页面或命令行输入为准 |

日常检查建议先运行 `smoke`；确认服务稳定后再使用 `standard`；需要更充分的报告材料时使用 `extended`。

### 审计对象

默认选择“内置 Docker 模型服务”，此时 Target API 与 Shadow API 指向本地容器服务，可以直接复现仓库内置流程。

如需接入外部模型，可选择“外部兼容 API”，并填写外部服务地址。Target 服务至少需要提供 label-only 分类接口；如果使用 `full` 模式，还需要可用的 Shadow 服务和匹配的阈值配置。

## 核心方法

### Label-Only 成员推理

成员推理攻击关注的问题是：一个样本是否曾被目标模型用于训练。训练集成员往往位于模型更熟悉的决策区域，预测行为可能更稳定、更接近模型记忆边界。Label-only 场景进一步限制攻击者只能看到分类标签，因此本项目不依赖目标模型概率分数，而是从“决策边界几何特征”和“影子模型行为特征”两条路径提取证据。

### 边界攻击

边界攻击用于估计样本到目标模型决策边界的距离。本项目基于 HopSkipJump Attack 思路，对原始样本和若干轻微扰动版本并发执行边界搜索，得到平均边界距离和局部波动系数。一般来说，若样本处在目标模型更稳定的区域，可能需要更大的扰动才会改变预测标签；若样本靠近边界，则更容易在扰动下发生类别变化。

为了让边界信号更适合样本级审计，系统会先选取一批确定未参与训练的“路人样本”做现场定标，统计其边界距离均值与波动情况，并据此生成动态距离阈值和稳定性阈值。这样可以减少不同模型、不同数据分布造成的固定阈值偏差。

### 影子模型信号

影子模型用于模拟目标模型的成员化行为。系统会读取 `shadow_config.json` 中的阈值配置，并根据样本在影子模型上的 loss 判断其成员风险。loss 越低，通常说明影子模型对该样本越熟悉；如果这种熟悉程度显著超过阈值，就会形成迁移攻击证据。

需要注意的是，影子模型不是越强越好。过度拟合的影子模型可能把所有训练标签都记住，导致 loss 接近 0，反而失去区分能力。本项目更关注影子模型是否能稳定模拟目标模型的决策轮廓。

### 双轨融合

单一信号容易受目标模型精度、样本难度、数据分布偏移和查询预算影响。本项目采用融合判定逻辑：当边界信号与 shadow loss 同时支持成员风险时，输出更高风险等级；当仅有单侧证据较强时，降级为疑似风险；当原图预测错误、边界距离异常或证据不足时，输出较低风险。

这种设计让报告不只给出一个结论，还能展示结论背后的证据来源，便于安全审计人员复核。

## 外部模型接入

本项目支持把外部分类模型封装成兼容的 HTTP 服务后接入。推荐接口如下：

```text
GET  /health
POST /predict
POST /predict_batch
POST /predict_logits
```

Target 服务至少需要实现：

```text
GET  /health
POST /predict
POST /predict_batch
```

Shadow 服务在 `full` 模式下需要实现：

```text
GET  /health
POST /predict_logits
```

请求体示例：

```json
{
  "image": [0.0, 0.1, 0.2]
}
```

返回体示例：

```json
{
  "label": 3,
  "logits": [0.1, 0.2, 1.8]
}
```

当前内置 CIFAR-10 示例使用长度为 `3072` 的展平图像向量。接入其他模型时，输入向量长度、归一化方式、标签空间和数据分布需要与外部模型保持一致。

完整接入外部模型通常分三步：

1. 将外部目标模型封装为兼容 Target API，并先用 `boundary-only` 验证调用稳定性。
2. 准备与目标任务同分布的数据，调用目标 API 生成标签或伪标签，形成影子模型训练数据。
3. 训练或校准 Shadow 服务，重新生成 `shadow_config.json`，再切换到 `full` 模式运行完整审计。

如果暂时无法训练影子模型，也可以只使用 `boundary-only` 模式进行初步风险扫描，但结论应保守解释。

## 常用命令

启动 Web 控制台：

```bash
docker compose up --build
```

运行一次容器内 CLI 审计：

```bash
docker compose --profile cli run --rm audit-runner
```

本地运行 Go Web 服务：

```bash
go run . --serve
```

本地运行快速审计：

```bash
go run . --preset smoke
```

指定外部 API 地址：

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
- Docker 默认安装 CPU 版 PyTorch，优先保证复现稳定性。需要 GPU 推理或重新训练模型时，可以在宿主机直接运行 `python_server/server.py`，或另行构建 CUDA 版镜像。

## License

本项目源码采用 MIT License 发布，详见 [LICENSE](LICENSE)。

第三方依赖、容器基础镜像、数据集与模型资产的许可说明见 [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md)。CIFAR-10 等第三方数据资产不由本项目许可证重新授权，使用者应遵循其原始发布条款。

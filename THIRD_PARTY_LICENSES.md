# Third-Party Licenses

This document summarizes the major third-party components used by LabelScan-Go.
The project source code is released under the MIT License. Third-party
libraries, datasets, model files, and toolchains remain governed by their own
licenses or terms.

This file is provided for release and compliance review. It is not legal advice.

## Go Components

The Go audit engine currently uses the Go standard library only. No external Go
modules are declared in `go.mod`.

| Component | Usage | License |
| --- | --- | --- |
| Go standard library | CLI, HTTP client/server, concurrency, JSON, statistics helpers | BSD-style Go license |

## Python Runtime Components

The Python oracle service and Docker image use the following major packages.

| Component | Usage | License |
| --- | --- | --- |
| Python | Runtime for the oracle service | Python Software Foundation License |
| FastAPI | HTTP API service for model inference | MIT |
| Uvicorn | ASGI server for FastAPI | BSD-3-Clause |
| NumPy | Tensor and numeric preprocessing helpers | BSD-3-Clause |
| PyTorch | Model loading and inference | BSD-style license |
| TorchVision | CIFAR-10 transforms and optional dataset utilities | BSD-3-Clause |
| Pydantic | Request and response validation, used through FastAPI | MIT |

## Optional Training And Evaluation Components

Some scripts under `python_server/`, `scripts/evaluation/`, and related
experiment utilities may use additional scientific Python packages. These are
not required by the minimal Go audit binary, but may be needed when retraining
models, recalculating thresholds, or reproducing plots.

| Component | Usage | License |
| --- | --- | --- |
| Pillow | Image loading and augmentation utilities | HPND / Pillow License |
| pandas | CSV and experiment result processing | BSD-3-Clause |
| SciPy | Statistical analysis utilities | BSD-3-Clause |
| Matplotlib | Evaluation plots | PSF-compatible Matplotlib License |
| Seaborn | Visualization helpers | BSD-3-Clause |

## Container Base Images

Docker builds are based on official language/runtime images.

| Component | Usage | License / Terms |
| --- | --- | --- |
| `python:3.11-slim` | Base image for PyTorch oracle services | Python image terms and Debian package licenses |
| `golang:1.24-bookworm` | Build image for the Go audit binary | Go image terms and Debian package licenses |
| `debian:bookworm-slim` | Runtime image for the Go audit service | Debian package licenses |

## Data And Model Assets

The repository contains data and model assets used for reproducible local
experiments. These assets should be reviewed separately from the project source
license.

| Asset | Usage | Notes |
| --- | --- | --- |
| CIFAR-10 binary and Python batch files | Local benchmark data for audit examples | CIFAR-10 is a third-party dataset. Users should follow the dataset provider's terms and citation requirements. |
| `python_server/CIFAR10/**/*.pth` | Target and shadow model checkpoints | Distributed for reproducibility with this project. They are not third-party source code dependencies. |
| `shadow_config.json`, `target_members.json` | Audit thresholds and member index metadata | Project-generated experiment metadata. |

## Release Checklist

Before publishing a formal release, maintainers should:

- Keep `requirements.txt`, Dockerfiles, and this document in sync.
- Recheck licenses when dependency versions are changed.
- Do not remove third-party copyright or license notices.
- Document any additional dataset, checkpoint, or external API dependency added
  in future releases.

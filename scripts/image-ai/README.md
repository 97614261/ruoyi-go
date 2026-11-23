# 本地图片 AI

用于图片处理节点的 `method: ai`。CPU 推理，无须 ComfyUI、GPU 或付费 Key。

- 抠图：rembg / U²-Net 轻量 u2netp。适合单主体，复杂发丝、半透明和多主体需人工检查。
- 高清放大：FSRCNN 2 倍/4 倍。真实神经网络超分，适合第一版轻量部署；不保证恢复原本不存在的细节。
- 普通处理不依赖这个服务；纯色去背景是边缘连通颜色移除，普通放大为 Catmull-Rom 插值。

## 安装（Windows，Python 3.10）

从项目根目录执行，所有环境和权重均放在忽略提交的 `.local`：

```powershell
& 'C:\Program Files\Python310\python.exe' -m venv .local/image-ai-venv
& .local/image-ai-venv/Scripts/python.exe -m pip install -r scripts/image-ai/requirements.txt
# rembg 依赖普通 OpenCV；最后重装 contrib，避免共用 cv2 命名空间覆盖超分模块。
& .local/image-ai-venv/Scripts/python.exe -m pip install --force-reinstall --no-deps opencv-contrib-python-headless==4.12.0.88
& .local/image-ai-venv/Scripts/python.exe scripts/image-ai/setup_models.py --models .local/image-ai-models
```

本机实测 ONNX Runtime 1.23.2 无法初始化，固定为 1.20.1。Python 可执行文件位置按实际安装位置调整。

在 `configs/pod.local.yml` 的 `pod.execution` 下增加：

```yaml
imageAIURL: http://127.0.0.1:18112
```

然后执行 `scripts/pod.ps1 -Action start`。开发启动脚本会先启动图片服务，再启动 API/Worker，并记录进程用于停止。
已有服务先 stop。清空该配置即可关闭 AI 选项；普通处理照常可用。

独立启动及验收：

```powershell
& .local/image-ai-venv/Scripts/python.exe scripts/image-ai/server.py --models .local/image-ai-models --port 18112
& .local/image-ai-venv/Scripts/python.exe scripts/image-ai/smoke.py --output .local/image-ai-smoke
```

## 边界

只监听回环地址，Go 客户端只接受 `http://127.0.0.1:端口`，禁用重定向和代理。仅接收图片字节，不接收图片 URL、路径或命令，不对浏览器开放跨域调用。不保存图片，不自动下载模型；启动时复核权重 SHA-256。

单次处理并发为 1，输入/输出最多 20 MiB、2400 万像素、8192 边长；超分分块推理限制峰值内存。Go 侧另受素材大小和任务超时限制，不能承诺任意大图都能处理。
此 HTTP 服务是本机 Worker 内部接口，不能直接映射到公网；部署跨机器方案需单独设计鉴权和 TLS。

权重安装器只下载固定地址并校验固定摘要。来源、大小与摘要记录在本地 `manifest.json`。

来源与许可信息：

- [rembg](https://github.com/danielgatis/rembg)（MIT），[U-2-Net](https://github.com/xuebinqin/U-2-Net)（Apache-2.0）。
- [FSRCNN TensorFlow](https://github.com/Saafke/FSRCNN_Tensorflow)（MIT），模型使用其 x2/x4 权重。
- [OpenCV](https://github.com/opencv/opencv) 与 [opencv_contrib](https://github.com/opencv/opencv_contrib)（Apache-2.0）。

这些权重和 Python 依赖不打进 Go 可执行文件。分发部署包时需同时安排运行环境、权重和对应许可文件。

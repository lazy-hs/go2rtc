# go2rtc 视频流模拟与 ONVIF 测试平台

本项目基于 [AlexxIT/go2rtc](https://github.com/AlexxIT/go2rtc) 进行功能扩展，保留其低延迟、多协议媒体网关能力，并面向视频算法联调、平台接入测试和 ONVIF 设备模拟场景，新增了一套中文可视化控制台。

通过控制台可以把本地视频、电脑摄像头或上游 RTSP 流统一接入 go2rtc，再以 RTSP、ONVIF、WebRTC、HLS、MP4、MJPEG 等方式对外提供服务。

> 本仓库包含针对业务场景的页面、接口、配置和发布流程改造。上游 go2rtc 的完整协议文档仍可参考 [官方仓库](https://github.com/AlexxIT/go2rtc) 和 [`internal/`](internal/README.md) 下的模块说明。

## 主要增强能力

| 模块       | 能力                                            |
| -------- | --------------------------------------------- |
| 视频流控制台   | 重构 `simulate.html`，集中展示任务、访问地址、消费者和运行资源       |
| 多种任务来源   | 支持本地文件、电脑摄像头和 RTSP 输入                         |
| 文件管理     | 支持后端文件浏览、本地文件上传、进度显示、取消上传和上传目录选择              |
| 任务管理     | 支持新建、编辑、删除、排序、启用、停用和自动状态刷新                    |
| 运行监控     | 展示 CPU、内存、Go 协程、运行时间、进程明细和消费者 IP              |
| ONVIF 模拟 | 支持设备信息、多个清晰度 Profile、RTSP 地址发布和 WS-Discovery  |
| 模拟 PTZ   | 支持绝对移动、相对移动、连续移动、停止、Home Position 和预置点        |
| ONVIF 事件 | 支持 PullPoint、Push Notify、事件模板、推送周期和持久订阅       |
| 版本管理     | 使用根目录 `VERSION` 作为唯一版本源，提供版本校验和自动递增工具         |
| 多架构发布    | 提供 PowerShell/Bash 构建脚本，输出多平台二进制和 SHA256 校验文件 |

## 快速开始

### 环境要求

- Go 1.24 或更高版本；
- FFmpeg，可通过系统环境变量找到，或按 [`internal/ffmpeg/README.md`](internal/ffmpeg/README.md) 配置；
- 使用摄像头时，需要允许进程访问本机采集设备；
- 使用 ONVIF 自动发现时，服务端与客户端通常需要位于同一网段。

### 从源码运行

在项目根目录执行：

```shell
go run .
```

go2rtc 默认读取当前目录的 `go2rtc.yaml`。也可以明确指定配置文件：

```shell
go run . -config /path/to/go2rtc.yaml
```

### 构建后运行

```shell
go build -trimpath
```

Windows：

```powershell
.\go2rtc.exe
```

Linux / macOS：

```bash
chmod +x ./go2rtc
./go2rtc
```

### 页面入口

当前仓库示例配置使用以下地址：

| 页面              | 地址                                    |
| --------------- | ------------------------------------- |
| go2rtc 原生 WebUI | `http://localhost:2984/`              |
| 视频流模拟与转发控制台     | `http://localhost:2984/simulate.html` |

当前 `go2rtc.yaml` 使用的端口如下：

| 服务                                      |     端口 | 说明                |
| --------------------------------------- | -----: | ----------------- |
| HTTP API / WebUI / ONVIF Device Service | `2984` | 页面、接口和 ONVIF 服务入口 |
| RTSP                                    | `9554` | 对外发布 RTSP 视频流     |
| WebRTC                                  | `9555` | WebRTC TCP/UDP 连接 |

如果替换了配置文件，请以其中的 `api.listen`、`rtsp.listen` 和 `webrtc.listen` 为准。未配置时，上游 go2rtc 的默认端口分别为 `1984`、`8554` 和 `8555`。

## 视频流模拟与转发控制台

控制台页面位于 [`www/simulate.html`](www/simulate.html)，主要用于管理模拟摄像机和转发任务。

### 页面概览

- 展示已配置任务数、活跃流、ONVIF 摄像机数量和停用任务数量；
- 汇总本地文件、电脑摄像头、RTSP 输入和消费者数量；
- 展示 go2rtc 与 FFmpeg 进程的 CPU、内存、PID、运行时间和 Go 协程；
- 展示每路流的来源、编码配置、运行状态、消费者和发布地址；
- 支持查看消费者 IP 明细和定位对应任务；
- 支持复制 RTSP、分档 RTSP 和 ONVIF 服务地址；
- 支持按配置顺序、名称和状态排序任务。

### 支持的任务来源

#### 本地文件

可以填写服务端可读取的绝对路径，也可以从浏览器上传本机文件。

- 后端文件浏览器可查看服务端可读取目录；
- 上传过程显示进度、已传大小和实时速度；
- 可以取消正在进行的上传；
- 单文件默认限制为 20 GiB；
- 文件先写入临时文件，上传完成后再原子发布，避免任务读取未完成文件；
- `api.upload_dir` 决定默认上传目录，未配置时使用 `static`；
- Windows 本机访问时可调用原生目录选择器；Linux、macOS、容器和远程访问使用网页目录树。

> 页面中的文件路径是 go2rtc 服务端路径，不是访问页面的客户端路径。容器部署时需要先正确挂载媒体目录。

#### 电脑摄像头

页面通过 `/api/ffmpeg/devices` 检测采集设备，并根据设备能力自动填入推荐分辨率和帧率。Windows 使用 DirectShow，Linux 通常使用 V4L2。

无摄像头的 Linux 服务器或容器会返回空设备列表，这是合法状态，不会阻止本地文件或 RTSP 任务运行。

#### RTSP 输入

可以填写上游 RTSP 地址，并配置认证信息、视频编码、音频处理和码率等参数。任务创建后会由 go2rtc 重新发布，方便测试下游平台、NVR 或算法服务。

### 任务启停与 Linux 只读配置

任务开关会同时更新运行时状态和 `simulate.disabled_streams`：

- 配置文件可写时，启停状态会持久化，服务重启后仍然有效；
- Linux 容器或只读文件系统中，如果配置无法写入，任务仍可在当前进程内临时启停；
- 临时变更时页面会提示“配置文件为只读，本次变更仅在当前服务运行期间生效”；
- 服务重启后，临时状态会恢复为配置文件中的状态。

启停接口必须使用 JSON 请求体：

```http
PUT /api/streams/state?src=demo
Content-Type: application/json

{"enabled": false}
```

如果只发送 `PUT /api/streams/state?src=demo` 而没有 JSON 请求体，接口会返回 `400 Bad Request`。

## 基础配置

下面的示例使用相对安全的演示路径，请按部署环境修改媒体文件位置和监听地址：

```yaml
ffmpeg:
  file: "-re -stream_loop -1 -i {input}"

api:
  listen: ":2984"
  upload_dir: "static"

rtsp:
  listen: ":9554"

webrtc:
  listen: ":9555"

streams:
  demo:
    - ffmpeg:/data/demo.mp4#video=h264#audio=aac#input=file

simulate:
  disabled_streams: []
  onvif_qualities:
    demo:
      - width: 0
        height: 0
      - width: 1280
        height: 720
```

发布地址示例：

```text
RTSP:  rtsp://127.0.0.1:9554/demo
ONVIF: http://127.0.0.1:2984/onvif/device_service
```

ONVIF 客户端会从设备服务中获取流列表和对应 RTSP 地址。RTSP 访问需要跨机器时，请把 `127.0.0.1` 替换为服务端可达 IP 或域名。

## ONVIF 模拟服务

### 设备信息

控制台可以配置模拟摄像机的名称、厂商、型号、固件、序列号、硬件名称、服务端口以及 RTSP 访问认证。保存设备信息后，服务会重启以应用监听端口和发布信息。

```yaml
onvif:
  name: "模拟摄像机"
  manufacturer: "Example"
  model: "Virtual IPC-9000"
  firmware: "2.1.0"
  serial: "CAM-001"
  hardware: "Virtual IPC"
```

这些信息会用于 `GetDeviceInformation`、`GetScopes` 和 WS-Discovery，使第三方 ONVIF 客户端看到一致的设备身份。

### 多清晰度 Profile

每路任务可以配置一个或多个 ONVIF 清晰度档位。`width: 0`、`height: 0` 表示保留原始分辨率，其他档位会作为独立 ONVIF Profile 发布。

```yaml
simulate:
  onvif_qualities:
    demo:
      - width: 0
        height: 0
      - width: 1920
        height: 1080
      - width: 1280
        height: 720
```

普通 RTSP 地址保持任务本身的编码和码率；ONVIF 客户端选择不同 Profile 时，服务按配置提供对应清晰度地址。

### 模拟 PTZ

PTZ 模拟支持：

- AbsoluteMove、RelativeMove、ContinuousMove 和 Stop；
- SetHomePosition 与 GotoHomePosition；
- 创建、读取、调用和删除预置点；
- 按流配置最大缩放倍数、水平/垂直/缩放速度和 Home Position；
- 在控制台全局启用或停用 PTZ，停用后立即停止相关派生转码以节省资源。

```yaml
simulate:
  ptz_enabled: true
  ptz:
    demo:
      max_zoom: 4
      pan_speed: 0.6
      tilt_speed: 0.6
      zoom_speed: 0.5
      home:
        pan: 0
        tilt: 0
        zoom: 0
```

### ONVIF 事件

事件服务支持 PullPoint 订阅和主动 Push Notify，包括：

- `CreatePullPointSubscription`、`PullMessages`；
- `Subscribe` 与 HTTP `Notify`；
- `Renew`、`GetStatus`、`SetSynchronizationPoint` 和 `Unsubscribe`；
- Topic 过滤、初始事件 Burst 和周期性事件；
- 中文预设事件模板以及自定义 Topic、数据和 PropertyOperation；
- 持久订阅模式，适合长时间稳定压测。

```yaml
event:
  enabled: true
  interval: 1m
  burst: 10
  permanent: true
  templates:
    - enabled: true
      name: 烟雾火焰
      topic: tns1:RuleEngine/FlameDetector
      startData: '<tt:SimpleItem Value="true" Name="IsMotion"/>'
      endData: '<tt:SimpleItem Value="false" Name="IsMotion"/>'
      startOperation: Changed
      endOperation: Deleted
```

页面内置烟雾火焰、区域闯入、越线、区域离开、物品丢失、人脸、人形、车辆、包裹、设备故障、镜头遮挡和设备移动等模板，也支持自定义事件。

更详细的 ONVIF 协议说明见 [`internal/onvif/README.md`](internal/onvif/README.md)。事件接收示例见 [`examples/onvif_event_receiver/`](examples/onvif_event_receiver/README.md)。

## HTTP API

控制台使用的主要接口如下。它们服务于当前页面实现，二次开发时请同时参考代码和 [`internal/api/README.md`](internal/api/README.md)。

| 接口                            | 方法             | 用途                    |
| ----------------------------- | -------------- | --------------------- |
| `/api/simulate`               | GET            | 获取控制台元数据、任务配置和实际接口地址  |
| `/api/streams`                | GET/PUT/DELETE | 查询、新建、编辑和删除视频流        |
| `/api/streams/state`          | GET/PUT        | 查询停用列表或启停指定任务         |
| `/api/simulate/files`         | GET            | 浏览后端文件和目录             |
| `/api/simulate/folder-picker` | POST           | 调用可用的原生目录选择器          |
| `/api/simulate/upload`        | POST           | 上传媒体文件                |
| `/api/ffmpeg/devices`         | GET            | 获取本机视频/音频采集设备         |
| `/api/simulate/metrics`       | GET            | 获取 CPU、内存、进程和 Go 运行指标 |
| `/api/simulate/onvif`         | GET/PUT        | 读取或保存 ONVIF 设备配置      |
| `/api/simulate/ptz`           | GET/PUT        | 查询或控制模拟 PTZ           |
| `/api/simulate/events`        | GET/PUT        | 读取或保存 ONVIF 事件配置      |
| `/api/restart`                | POST           | 保存关键配置后重启当前进程         |

如果配置了 `api.base_path`，页面会自动使用带前缀的接口地址，不应在前端硬编码 `/api/...`。

## 版本号管理

根目录 [`VERSION`](VERSION) 是应用版本的唯一来源，格式为 `M.m.p`。构建时版本号会嵌入二进制，发布标签必须使用 `vM.m.p` 并与文件内容完全一致。

### 版本变更规则

- 常规迭代 `regular`：次版本号加一，修订号归零；
- 专项迭代 `special`：修订号加一；
- 重大升级 `major`：主版本号加一，次版本号和修订号归零；
- 次版本号只允许 `0-9`，从 `1.9.x` 开始常规迭代时进位为 `2.0.0`；
- 架构、核心技术栈或 API 出现不兼容变更时，应使用重大升级。

### 版本命令

```shell
# 查看和校验当前版本
go run ./cmd/version show
go run ./cmd/version check

# 预览下一版本，不修改文件
go run ./cmd/version next regular
go run ./cmd/version next special
go run ./cmd/version next major

# 更新 VERSION
go run ./cmd/version bump regular
go run ./cmd/version bump special
go run ./cmd/version bump major
```

GitHub Actions 会在构建二进制和镜像前校验版本格式，并在标签发布时检查标签与 `VERSION` 是否一致。完整规则和发布流程见 [`VERSIONING.md`](VERSIONING.md)。

## 多架构构建

构建脚本位于 [`packaging/`](packaging/README.md)，默认使用：

```text
CGO_ENABLED=0
go build -ldflags "-s -w" -trimpath
```

### PowerShell

```powershell
# 构建全部目标
.\packaging\build.ps1

# 构建指定目标
.\packaging\build.ps1 linux-amd64

# 查看支持的目标
.\packaging\build.ps1 -List
```

### Bash

```bash
chmod +x packaging/build.sh

# 构建全部目标
./packaging/build.sh

# 构建指定目标
./packaging/build.sh linux-amd64

# 查看支持的目标
./packaging/build.sh --list
```

当前支持 Windows、Linux、macOS 和 FreeBSD 的常用 AMD64、386、ARM64、ARMv6/ARMv7、MIPSLE 等目标。产物写入 `packaging/dist/`，并生成 `SHA256SUMS.txt`。

详细目标列表、构建前测试和 Linux 二进制诊断方法见：

- [`packaging/README.md`](packaging/README.md)
- [`多架构打包说明.md`](多架构打包说明.md)

## Linux 与 Docker 注意事项

- 如果需要永久保存页面上的任务启停、编辑、ONVIF 或事件配置，必须保证 `go2rtc.yaml` 可写；
- 配置只读时仍可临时启停任务，但重启后恢复配置文件状态；
- 容器内的媒体文件路径必须通过 Volume 映射；
- 摄像头设备需要映射 `/dev/video*` 等设备，并授予相应权限；
- ONVIF WS-Discovery 使用 UDP `3702` 和组播 `239.255.255.250`，Docker 中通常建议使用 host 网络；
- 对外开放 WebUI、文件浏览或上传能力前，应配置 API 认证和网络访问控制；
- RTSP、WebRTC 和 ONVIF 端口需要在防火墙或容器编排中正确放行。

API 认证示例：

```yaml
api:
  listen: ":2984"
  username: "admin"
  password: "change-me"
  local_auth: true
  upload_dir: "/data/uploads"
```

> 文件浏览可以查看服务进程有权限读取的目录，上传目录选择可以写入用户选中的现有目录。不要把未鉴权的控制台暴露到不可信网络。

## 常见问题

### `/api/ffmpeg/devices` 返回 404

通常表示前端页面与后端二进制版本不匹配，或配置中的 `app.modules` 没有启用 `ffmpeg`。请使用当前源码重新构建，并确认模块列表包含 `api`、`ffmpeg` 和页面所需协议模块。

### `/api/streams/state?src=...` 返回 400

启停请求必须是 `PUT`，并携带 JSON 请求体 `{"enabled": true}` 或 `{"enabled": false}`。直接访问 URL 或使用旧版页面会因为缺少请求体而返回 400。

### Linux 中无法停止视频流任务

当前实现允许在配置只读时进行运行期启停。请确认页面与二进制都来自当前版本。若响应中的 `persisted` 为 `false`，说明停流已经生效，但只持续到服务重启；如需持久化，请为配置文件提供写权限。

### 页面没有检测到摄像头

检查 FFmpeg 是否可执行、系统是否授予设备权限，以及容器是否映射了采集设备。无设备的服务器会正常返回空列表，不属于接口故障。

### ONVIF 客户端搜索不到设备

确认客户端与服务端位于同一网段，放行 UDP 3702 和组播流量。Docker 部署优先使用 host 网络，同时检查客户端是否允许手动添加 `http://<host>:<api-port>/onvif/device_service`。

## 上游 go2rtc 核心能力

在本项目增强功能之外，仍保留上游 go2rtc 的主要能力：

- RTSP、RTMP、WebRTC、HLS、HTTP、MJPEG、ONVIF、HomeKit 等输入或输出；
- H.264、H.265、MJPEG、AAC、Opus、PCMA、PCMU 等常见视频与音频编码；
- 按需通过 FFmpeg 转码，支持多种软硬件编码器；
- 双向音频、多个来源轨道组合、预加载和流发布；
- WebSocket API、流状态统计和多种网页播放器；
- 可作为独立服务，也可以集成到 Home Assistant 或其他平台。

模块与协议的详细说明：

- [`internal/README.md`](internal/README.md)：模块总览
- [`internal/streams/README.md`](internal/streams/README.md)：流配置与管理
- [`internal/ffmpeg/README.md`](internal/ffmpeg/README.md)：FFmpeg 输入、转码和参数
- [`internal/rtsp/README.md`](internal/rtsp/README.md)：RTSP 客户端与服务端
- [`internal/webrtc/README.md`](internal/webrtc/README.md)：WebRTC
- [`www/README.md`](www/README.md)：原生 WebUI 和前端 API

## 项目结构

```text
.
├── VERSION                 # 唯一版本号来源
├── VERSIONING.md           # 版本规则与发布流程
├── go2rtc.yaml             # 当前运行配置
├── cmd/version/            # 版本管理命令
├── internal/               # go2rtc 核心模块与新增后端接口
├── www/simulate.html       # 视频流模拟与转发控制台
├── packaging/              # 多架构构建脚本和说明
├── examples/               # ONVIF 等示例程序
└── website/                # 上游网站资源
```

## 致谢与许可证

本项目建立在 [AlexxIT/go2rtc](https://github.com/AlexxIT/go2rtc) 及其依赖的开源生态之上，感谢上游作者、Pion WebRTC 团队、FFmpeg 社区以及所有贡献者。

许可证见 [`LICENSE`](LICENSE)。使用和分发本项目时，请同时遵守上游项目及相关第三方组件的许可证要求。

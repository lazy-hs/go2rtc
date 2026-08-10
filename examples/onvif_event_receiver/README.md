# ONVIF 事件接收验证工具

该工具作为独立 ONVIF 客户端运行，用于验证摄像机或 go2rtc 是否按照 ONVIF 标准产生并发送事件。

支持两种模式：

- `push`：默认模式。工具发送 `Subscribe`，并启动 HTTP 回调接收摄像机主动发送的 `Notify`。
- `pull`：发送 `CreatePullPointSubscription`，循环调用 `PullMessages` 拉取事件，用于和主动推送结果对照。

## 构建

Windows：

```powershell
go build -o onvif_event_receiver.exe ./examples/onvif_event_receiver
```

Linux AMD64：

```powershell
$env:CGO_ENABLED="0"
$env:GOOS="linux"
$env:GOARCH="amd64"
go build -o onvif_event_receiver_linux_amd64 ./examples/onvif_event_receiver
```

## 主动推送验证

```powershell
./onvif_event_receiver.exe `
  -device "onvif://admin:password@192.168.1.64/onvif/device_service" `
  -mode push `
  -listen ":18080"
```

工具会根据访问摄像机的网络路由自动生成回调地址，例如：

```text
Notify callback: http://192.168.1.10:18080/onvif/notify
```

摄像机必须能够访问这个 IP 和端口。存在多网卡、Docker、NAT 或防火墙时，应显式指定摄像机可访问的地址：

```powershell
./onvif_event_receiver.exe `
  -device "http://admin:password@192.168.1.64/onvif/device_service" `
  -mode push `
  -listen ":18080" `
  -callback "http://192.168.1.10:18080/onvif/notify" `
  -topic "tns1:VideoSource/MotionAlarm" `
  -raw
```

收到事件时，标准输出为一行一个 JSON，例如：

```json
{"type":"onvif_event","mode":"push","received_at":"2026-08-06T10:00:00Z","topic":"tns1:VideoSource/MotionAlarm","utc_time":"2026-08-06T09:59:59Z","property_operation":"Changed","source":[{"name":"VideoSourceConfigurationToken","value":"main"}],"data":[{"name":"IsMotion","value":"true"}]}
```

只有输出中的 `"mode":"push"` 且日志显示 `Notify received`，才能证明事件是摄像机主动推送的。

## PullPoint 对照验证

```powershell
./onvif_event_receiver.exe `
  -device "http://admin:password@192.168.1.64/onvif/device_service" `
  -mode pull `
  -pull-timeout 15s `
  -message-limit 100
```

## 常用参数

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-device` | 无 | ONVIF 设备服务 URL，必填 |
| `-event-url` | 自动发现 | 手动覆盖 Events 服务地址 |
| `-mode` | `push` | `push` 或 `pull` |
| `-listen` | `:18080` | Push 回调监听地址 |
| `-callback` | 自动生成 | 摄像机访问的完整回调 URL |
| `-topic` | 空 | ONVIF ConcreteSet Topic 过滤器 |
| `-duration` | `0` | 运行时长，`0` 表示直到 Ctrl+C |
| `-subscription-ttl` | `10m` | 请求的订阅有效期，工具会自动续订 |
| `-pull-timeout` | `15s` | 单次 PullMessages 等待时间 |
| `-message-limit` | `100` | 单次 PullMessages 最大事件数 |
| `-raw` | `false` | 在标准错误中打印原始 SOAP XML |

工具同时发送 WS-Security UsernameToken，并支持摄像机返回 HTTP Digest 认证挑战。退出时会发送 `Unsubscribe` 清理订阅。

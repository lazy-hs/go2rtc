# go2rtc 多架构打包脚本

本目录集中存放多架构构建脚本，构建后的可执行文件统一输出到 `packaging/dist/`。

## 目录结构

```text
packaging/
├── build.cmd       # Windows 快捷入口
├── build.ps1       # Windows PowerShell
├── build.sh        # Linux / macOS Bash
├── README.md
└── dist/           # 可执行文件与 SHA256SUMS.txt
```

脚本统一使用以下官方构建参数：

```text
CGO_ENABLED=0
go build -ldflags "-s -w" -trimpath
```

`amd64` 目标会额外固定 `GOAMD64=v1`，以兼容最基础的 x86-64 指令集。构建完成后脚本会检查产物中的 `GOOS`、`GOARCH`、`CGO_ENABLED` 和 `GOAMD64` 元数据，避免环境变量污染或误用旧产物。

默认会先运行 `go test -count=1 ./internal/api`，再执行构建。可使用 `-SkipTest` 或 `--skip-test` 跳过测试。

## PowerShell 用法

在项目根目录执行。若系统限制执行 `.ps1`，可使用下面这种方式：

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File .\packaging\build.ps1
```

也可以直接使用不受 PowerShell 执行策略影响的快捷入口：

```cmd
packaging\build.cmd linux-amd64
```

不要在 `packaging/` 目录中直接执行 `go build .` 或 `go test ./internal/api`；`.` 和相对包路径会指向打包目录。若当前位于 `packaging/`，请先执行 `Set-Location ..` 返回项目根目录。

常用示例：

```powershell
# 构建全部架构
.\packaging\build.ps1

# 只构建全部 Linux 架构
.\packaging\build.ps1 linux

# 构建指定架构
.\packaging\build.ps1 linux-amd64, linux-arm64, windows-amd64

# 清空 dist 后构建，并跳过测试
.\packaging\build.ps1 linux-amd64 -Clean -SkipTest

# 查看支持的目标
.\packaging\build.ps1 -List
```

## Bash 用法

```bash
chmod +x packaging/build.sh

# 构建全部架构
./packaging/build.sh

# 只构建全部 Linux 架构
./packaging/build.sh linux

# 构建指定架构
./packaging/build.sh linux-amd64 linux-arm64 windows-amd64

# 清空 dist 后构建，并跳过测试
./packaging/build.sh --clean --skip-test linux-amd64

# 查看支持的目标
./packaging/build.sh --list
```

## 支持的目标

| 目标 | 输出文件 |
| --- | --- |
| `windows-amd64` | `go2rtc_win64.exe` |
| `windows-386` | `go2rtc_win32.exe` |
| `windows-arm64` | `go2rtc_win_arm64.exe` |
| `linux-amd64` | `go2rtc_linux_amd64` |
| `linux-386` | `go2rtc_linux_i386` |
| `linux-arm64` | `go2rtc_linux_arm64` |
| `linux-armv7` | `go2rtc_linux_arm` |
| `linux-armv6` | `go2rtc_linux_armv6` |
| `linux-mipsle` | `go2rtc_linux_mipsel` |
| `darwin-amd64` | `go2rtc_mac_amd64` |
| `darwin-arm64` | `go2rtc_mac_arm64` |
| `freebsd-amd64` | `go2rtc_freebsd_amd64` |
| `freebsd-arm64` | `go2rtc_freebsd_arm64` |

也可以使用分组目标：`all`、`windows`、`linux`、`darwin`、`freebsd`。`mac` 和 `macos` 是 `darwin` 的别名。

## 构建产物

每次构建会在 `packaging/dist/` 中生成：

- 所选平台的可执行文件；
- `SHA256SUMS.txt` 校验清单。

脚本不会复制 `go2rtc.yaml`、上传文件或视频文件，避免把本地路径、凭据或业务数据带入发布产物。部署时请按目标环境单独准备配置和静态文件。

## Linux 运行诊断

将产物复制到 Linux 后，先检查架构、权限和文件校验值：

```bash
uname -m
uname -r
file ./go2rtc_linux_amd64
chmod +x ./go2rtc_linux_amd64
sha256sum ./go2rtc_linux_amd64
GOTRACEBACK=crash ./go2rtc_linux_amd64 -v
```

- `uname -m` 为 `x86_64` 时使用 `linux-amd64`；
- `aarch64` 使用 `linux-arm64`；
- `armv7l` 使用 `linux-armv7`；
- `i386` 或 `i686` 使用 `linux-386`。

若仍显示 `Segmentation fault`，继续执行 `dmesg | tail -n 30`（可能需要 root），并对照 `packaging/dist/SHA256SUMS.txt`。没有 Go 堆栈、只在 `dmesg` 中出现崩溃时，通常是目标架构、模拟器或旧内核兼容问题，而不是配置文件解析错误。

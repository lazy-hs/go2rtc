# Docker

Images are built automatically via [GitHub actions](https://github.com/AlexxIT/go2rtc/actions) and published on [Docker Hub](https://hub.docker.com/r/alexxit/go2rtc) and [GitHub](https://github.com/AlexxIT/go2rtc/pkgs/container/go2rtc).

## Versions

- `alexxit/go2rtc:latest` - latest release based on `alpine` (`amd64`, `386`, `arm/v6`, `arm/v7`, `arm64`) with support for hardware transcoding for Intel iGPU and Raspberry
- `alexxit/go2rtc:latest-hardware` - latest release based on `debian 13` (`amd64`) with support for hardware transcoding for Intel iGPU, AMD GPU and NVidia GPU
- `alexxit/go2rtc:latest-rockchip` - latest release based on `debian 12` (`arm64`) with support for hardware transcoding for Rockchip RK35xx
- `alexxit/go2rtc:master` - latest unstable version based on `alpine`
- `alexxit/go2rtc:master-hardware` - latest unstable version based on `debian 13` (`amd64`)
- `alexxit/go2rtc:master-rockchip` - latest unstable version based on `debian 12` (`arm64`)

## Docker compose

```yaml
services:
  go2rtc:
    image: alexxit/go2rtc
    network_mode: host       # important for WebRTC, HomeKit, UDP cameras
    privileged: true         # only for FFmpeg hardware transcoding
    restart: unless-stopped  # autorestart on fail or config change from WebUI
    environment:
      - TZ=Atlantic/Bermuda  # timezone in logs
    volumes:
      - "~/go2rtc:/config"   # go2rtc.yaml and go2rtc_linux.yaml
```

## Basic Deployment

```bash
docker run -d \
  --name go2rtc \
  --network host \
  --privileged \
  --restart unless-stopped \
  -e TZ=Atlantic/Bermuda \
  -v ~/go2rtc:/config \
  alexxit/go2rtc
```

Linux Docker deployments use both `/config/go2rtc.yaml` and
`/config/go2rtc_linux.yaml`. Keep both files in the mounted directory when
using the repository's platform-specific stream configuration.

## Deployment with GPU Acceleration

```bash
docker run -d \
  --name go2rtc \
  --network host \
  --privileged \
  --restart unless-stopped \
  -e TZ=Atlantic/Bermuda \
  --gpus all \
  -v ~/go2rtc:/config \
  alexxit/go2rtc:latest-hardware
```

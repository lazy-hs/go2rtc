# ONVIF

## ONVIF Client

[`new in v1.5.0`](https://github.com/AlexxIT/go2rtc/releases/tag/v1.5.0)

The source is not very useful if you already know RTSP and snapshot links for your camera. But it can be useful if you don't.

**WebUI > Add** webpage supports ONVIF autodiscovery. Your server must be on the same subnet as the camera. If you use Docker, you must use "network host".

```yaml
streams:
  dahua1: onvif://admin:password@192.168.1.123
  reolink1: onvif://admin:password@192.168.1.123:8000
  tapo1: onvif://admin:password@192.168.1.123:2020
```

## ONVIF Server

A regular camera has a single video source (`GetVideoSources`) and two profiles (`GetProfiles`).

Go2rtc has one video source and one profile per stream.

The simulated camera identity can be configured. These values are used by
`GetDeviceInformation`, `GetScopes`, and WS-Discovery so clients see the same
name and model in every view.

```yaml
onvif:
  name: "Front Door Camera"
  manufacturer: "ACME"
  model: "Virtual IPC-9000"
  firmware: "2.1.0"
  serial: "CAM-001"
  hardware: "Virtual IPC"
```

All fields are optional. The firmware defaults to the running go2rtc version,
and the serial number defaults to the request host when omitted.

`GetNetworkInterfaces` reports the name, MAC address, MTU, and IPv4 settings of
the local interface used by the ONVIF client connection. When go2rtc runs in a
container, the reported MAC address belongs to the container network namespace.

The server listens for WS-Discovery Probe requests on
`239.255.255.250:3702` and replies with the device service address of the
network interface that received the request.

### ONVIF events

The ONVIF server supports PullPoint subscriptions with `CreatePullPointSubscription`
and `PullMessages`, plus active WS-BaseNotification delivery with `Subscribe` and
HTTP `Notify`. Both subscription modes support `Renew`, `GetStatus`,
`SetSynchronizationPoint`, topic filters, and `Unsubscribe`.

The advertised Events endpoint is `/onvif/event`. The legacy
`/onvif/event_service` endpoint remains accepted. PullPoint subscription
manager addresses use `/onvif/Subscription?Idx=...` for compatibility with
ONVIF clients that use this address format.

```yaml
event:
  interval: 1m
  burst: 10
  templates:
    - topic: tns1:VideoSource/MotionAlarm
      sourceData: '<tt:SimpleItem Value="analytics_video_source_audio_source" Name="VideoAnalyticsConfigurationToken"/><tt:SimpleItem Value="MyMotionDetectorRule" Name="Rule"/>'
      startData: '<tt:SimpleItem Value="true" Name="IsMotion"/>'
      endData: '<tt:SimpleItem Value="false" Name="IsMotion"/>'
      startOperation: Changed
      endOperation: Deleted
```

`sourceData`, `startOperation`, and `endOperation` are optional. When an ONVIF
client calls `SetSynchronizationPoint`, the generated synchronization events
use `PropertyOperation="Initialized"`.

## Tested clients

Go2rtc works as ONVIF server:

- Happytime onvif client (windows)
- Home Assistant ONVIF integration (linux)
- Onvier (android)
- ONVIF Device Manager (windows)

PS. Supports only TCP transport for RTSP protocol. UDP and HTTP transports - unsupported yet.

## Tested cameras

Go2rtc works as ONVIF client:

- Dahua IPC-K42
- OpenIPC
- Reolink RLC-520A
- TP-Link Tapo TC60

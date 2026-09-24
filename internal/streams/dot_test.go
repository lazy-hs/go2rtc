package streams

import (
	"strings"
	"testing"
)

func TestAppendDOTLocalPipeUsesSemanticHost(t *testing.T) {
	c := &conn{
		ID:         1,
		FormatName: "rtsp",
		Protocol:   "pipe",
		BytesRecv:  1234,
	}

	dot := string(c.appendDOT(nil, "producer"))

	if strings.Contains(dot, "127.0.0.1") {
		t.Fatalf("local pipe must not be presented as a network IP:\n%s", dot)
	}
	if !strings.Contains(dot, `"local-pipe" [group=host, label="本地进程", title="本地管道连接，无远程 IP"]`) {
		t.Fatalf("local pipe host should have a semantic label:\n%s", dot)
	}
	if !strings.Contains(dot, `"local-pipe" -> 1`) {
		t.Fatalf("producer edge should start at the semantic local host:\n%s", dot)
	}
}

func TestAppendDOTRemoteHostKeepsIPAddress(t *testing.T) {
	c := &conn{
		ID:         2,
		FormatName: "rtsp",
		Protocol:   "tcp",
		RemoteAddr: "192.168.1.20:554",
	}

	dot := string(c.appendDOT(nil, "producer"))

	if !strings.Contains(dot, `"192.168.1.20" [group=host]`) {
		t.Fatalf("remote connection should keep its real IP address:\n%s", dot)
	}
}

func TestAppendDOTLoopbackRTSPUsesSemanticHost(t *testing.T) {
	c := &conn{
		ID:         3,
		FormatName: "rtsp",
		Protocol:   "tcp",
		RemoteAddr: "127.0.0.1:8554",
	}

	dot := string(c.appendDOT(nil, "producer"))

	if strings.Contains(dot, "127.0.0.1") {
		t.Fatalf("loopback RTSP must not be presented as a remote IP:\n%s", dot)
	}
	if !strings.Contains(dot, `"local-loopback" [group=host, label="本地进程", title="本地回环连接，无远程 IP"]`) {
		t.Fatalf("loopback RTSP should have a semantic label:\n%s", dot)
	}
}

package rtsp

import (
	"io"
	"net"
	"testing"

	"github.com/AlexxIT/go2rtc/pkg/core"
	"github.com/pion/rtp"
)

func TestPacketWriterCountsSuccessfulWrites(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	c := &Conn{
		conn:   server,
		state:  StatePlay,
		playOK: true,
	}
	handler := c.packetWriter(&core.Codec{
		Name:        core.CodecH264,
		ClockRate:   90000,
		PayloadType: 96,
	}, 0, 96)

	payload := []byte{1, 2, 3, 4}
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 4+12+len(payload))
		_, err := io.ReadFull(client, buf)
		readDone <- err
	}()

	handler(&rtp.Packet{
		Header:  rtp.Header{Version: 2, Marker: true},
		Payload: payload,
	})

	if err := <-readDone; err != nil {
		t.Fatalf("read interleaved RTP packet: %v", err)
	}

	if want := 4 + 12 + len(payload); c.Send != want {
		t.Fatalf("connection send bytes = %d, want %d", c.Send, want)
	}
}

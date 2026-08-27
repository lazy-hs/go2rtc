package rtsp

import (
	"net"
	"testing"

	pkgRTSP "github.com/AlexxIT/go2rtc/pkg/rtsp"
	"github.com/stretchr/testify/require"
)

func TestSetRTSPEnabledClosesExistingServerConnections(t *testing.T) {
	previous := RTSPEnabled()
	SetRTSPEnabled(true)
	t.Cleanup(func() { SetRTSPEnabled(previous) })

	serverSide, clientSide := net.Pipe()
	t.Cleanup(func() { _ = clientSide.Close() })
	conn := pkgRTSP.NewServer(serverSide)
	registerRTSPServerConn(conn)
	t.Cleanup(func() { unregisterRTSPServerConn(conn) })

	SetRTSPEnabled(false)
	require.False(t, RTSPEnabled())
	_, err := clientSide.Read(make([]byte, 1))
	require.Error(t, err)
}

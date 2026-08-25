package device

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmptyDeviceSourcesResponse(t *testing.T) {
	res := httptest.NewRecorder()

	writeDeviceSources(res, nil)

	require.Equal(t, 200, res.Code)
	require.JSONEq(t, `{"sources":[]}`, res.Body.String())
}

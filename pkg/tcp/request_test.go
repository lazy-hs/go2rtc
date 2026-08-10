package tcp

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoDigestRetryRestoresBody(t *testing.T) {
	const payload = `<Envelope><Body>test</Body></Envelope>`
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Digest ") {
			w.Header().Set("WWW-Authenticate", `Digest realm="camera", nonce="nonce", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.Equal(t, payload, string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	previousClient := client
	client = server.Client()
	defer func() { client = previousClient }()

	u, err := url.Parse(server.URL)
	require.NoError(t, err)
	u.User = url.UserPassword("admin", "secret")
	req, err := http.NewRequest(http.MethodPost, u.String(), bytes.NewReader([]byte(payload)))
	require.NoError(t, err)

	res, err := Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	require.Equal(t, http.StatusOK, res.StatusCode)
	require.Equal(t, 2, requests)
}

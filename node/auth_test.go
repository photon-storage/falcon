package node

import (
	gohttp "net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/photon-storage/go-common/testing/require"
)

func TestRedirect(t *testing.T) {
	path := "/ipfs/mock_cid/file.txt?arg=value"
	original := "http://127.0.0.1:8080" + path
	cases := []struct {
		name     string
		starbase string
		expected string
	}{
		{
			name:     "http",
			starbase: "http://192.168.0.1",
			expected: "http://192.168.0.1" + path,
		},
		{
			name:     "https",
			starbase: "https://192.168.0.1",
			expected: "https://192.168.0.1" + path,
		},
		{
			name:     "no scheme",
			starbase: "https://192.168.0.1",
			expected: "https://192.168.0.1" + path,
		},
		{
			name:     "http with port",
			starbase: "http://192.168.0.1:8080",
			expected: "http://192.168.0.1:8080" + path,
		},
		{
			name:     "https with port",
			starbase: "https://192.168.0.1:8080",
			expected: "https://192.168.0.1:8080" + path,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, err := gohttp.NewRequest(gohttp.MethodGet, original, nil)
			require.NoError(t, err)

			target, err := url.Parse(c.starbase)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			redirectToStarbase(w, r, target)

			require.Equal(t, gohttp.StatusTemporaryRedirect, w.Code)
			u, err := url.Parse(w.Header().Get("Location"))
			require.NoError(t, err)
			require.Equal(t, c.expected, u.String())
		})
	}
}

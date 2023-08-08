package node

import (
	gohttp "net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/photon-storage/go-common/testing/require"

	"github.com/photon-storage/falcon/node/config"
)

func TestRedirect(t *testing.T) {
	hostname := "xxx.gtw3.io"
	path := "/ipfs/mock_cid/file.txt?arg=value"
	cases := []struct {
		name     string
		hostname string
		original string
		starbase string
		expected string
	}{
		{
			name:     "http",
			original: "http://" + hostname + path,
			starbase: "http://gw3.io",
			expected: "http://gw3.io" + path,
		},
		{
			name:     "https",
			original: "https://" + hostname + path,
			starbase: "https://gw3.io",
			expected: "https://gw3.io" + path,
		},
		{
			name:     "http with port",
			original: "http://" + hostname + ":8080" + path,
			starbase: "http://gw3.io",
			expected: "http://gw3.io" + path,
		},
		{
			name:     "https with port",
			original: "https://" + hostname + ":8080" + path,
			starbase: "https://gw3.io",
			expected: "https://gw3.io" + path,
		},
		{
			name:     "https with subdomain and port",
			original: "https://mock_cid.ipfs." + hostname + ":8080" + path,
			starbase: "https://gw3.io",
			expected: "https://mock_cid.ipfs.gw3.io" + path,
		},
		{
			name:     "https with subdomain and port",
			original: "https://mock_cid.ipns." + hostname + ":8080" + path,
			starbase: "https://gw3.io",
			expected: "https://mock_cid.ipns.gw3.io" + path,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := config.Config{}
			cfg.GW3Hostname = hostname
			cfg.ExternalServices.Starbase = c.starbase
			config.Mock(&cfg)

			r, err := gohttp.NewRequest(gohttp.MethodGet, c.original, nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			redirectToStarbase(w, r)

			require.Equal(t, gohttp.StatusTemporaryRedirect, w.Code)
			u, err := url.Parse(w.Header().Get("Location"))
			require.NoError(t, err)
			require.Equal(t, c.expected, u.String())
		})
	}
}

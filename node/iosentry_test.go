package node

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/photon-storage/go-common/testing/require"
)

func TestCheckSentryRule(t *testing.T) {
	require.Equal(t, "", checkSentryRule([]byte("this is test")))

	for _, rl := range rules {
		require.Equal(t,
			rl.name,
			checkSentryRule(append(rl.exact, []byte("test")...)),
		)
		require.Equal(t,
			rl.name,
			checkSentryRule(append([]byte("test"), rl.exact...)),
		)
		require.Equal(t,
			rl.name,
			checkSentryRule(append(
				append([]byte("test prefix"), rl.exact...),
				[]byte("test suffix")...,
			)),
		)
	}
}

func TestIOSentry(t *testing.T) {
	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "no detection",
			run: func(t *testing.T) {
				for _, rl := range rules {
					w := httptest.NewRecorder()
					s := newContentSentry(w)
					s.Header().Set("Content-Type", "text/plain")
					s.WriteHeader(http.StatusOK)
					data := append(rl.exact, []byte("test")...)
					n, err := s.Write(data)
					require.NoError(t, err)
					require.Equal(t, len(data), n)
					require.Equal(t, http.StatusOK, w.Code)
					require.DeepEqual(t, data, w.Body.Bytes())
				}
			},
		},
		{
			name: "flagged in first batch",
			run: func(t *testing.T) {
				for _, rl := range rules {
					w := httptest.NewRecorder()
					s := newContentSentry(w)
					s.Header().Set("Content-Type", "text/html")
					s.WriteHeader(http.StatusOK)

					n, err := s.Write(rl.exact)
					require.NoError(t, err)
					require.Equal(t, len(rl.exact), n)

					n, err = s.Write(bytes.Repeat(
						[]byte("a"),
						batchSize-len(rl.exact)),
					)
					require.Equal(t, 0, n)
					require.ErrorIs(t, ErrBadContent, err)

					n, err = s.Write(bytes.Repeat([]byte("a"), 100))
					require.Equal(t, 0, n)
					require.ErrorIs(t, ErrBadContent, err)
					require.NoError(t, s.flush())

					require.Equal(t, http.StatusGone, w.Code)
					require.Equal(t, 0, len(w.Body.Bytes()))
					require.Equal(t, rl.name, s.getFlaggedRuleName())
				}
			},
		},
		{
			name: "flagged in following batch",
			run: func(t *testing.T) {
				for _, rl := range rules {
					w := httptest.NewRecorder()
					s := newContentSentry(w)
					s.Header().Set("Content-Type", "text/html")
					s.WriteHeader(http.StatusOK)

					n, err := s.Write(bytes.Repeat([]byte("a"), batchSize))
					require.NoError(t, err)
					require.Equal(t, batchSize, n)

					n, err = s.Write(rl.exact)
					require.NoError(t, err)
					require.Equal(t, len(rl.exact), n)

					n, err = s.Write(bytes.Repeat(
						[]byte("a"),
						batchSize-len(rl.exact)),
					)
					require.Equal(t, 0, n)
					require.ErrorIs(t, ErrBadContent, err)

					n, err = s.Write(bytes.Repeat([]byte("a"), 100))
					require.Equal(t, 0, n)
					require.ErrorIs(t, ErrBadContent, err)
					require.NoError(t, s.flush())

					require.Equal(t, http.StatusOK, w.Code)
					require.DeepEqual(t,
						bytes.Repeat([]byte("a"), batchSize),
						w.Body.Bytes(),
					)
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

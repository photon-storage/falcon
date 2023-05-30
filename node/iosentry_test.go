package node

import (
	"net/http/httptest"
	"testing"

	"github.com/photon-storage/go-common/testing/require"
)

func TestIOSentry(t *testing.T) {
	for _, rl := range rules {
		w := httptest.NewRecorder()
		s := newContentSentry(w)
		n, err := s.Write([]byte("test data 1"))
		require.NoError(t, err)
		require.Equal(t, 11, n)
		n, err = s.Write([]byte("test data 2"))
		require.NoError(t, err)
		require.Equal(t, 11, n)

		_, err = s.Write(append(rl.exact, []byte("test")...))
		require.ErrorIs(t, ErrBadContent, err)

		_, err = s.Write([]byte("test data 3"))
		require.ErrorIs(t, ErrBadContent, err)
	}
}

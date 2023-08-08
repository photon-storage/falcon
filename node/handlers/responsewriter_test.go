package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/photon-storage/go-common/testing/require"
)

type mockHandler struct {
	code int
}

func (h *mockHandler) status(code int) {
	h.code = code
}

func (h *mockHandler) update(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}

func TestResponseWriter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cases := []struct {
		name string
		run  func(t *testing.T)
	}{
		{
			name: "no callback",
			run: func(t *testing.T) {
				w := httptest.NewRecorder()
				rw := newResponseWriter(ctx, w, nil)

				data := []byte("test data")
				rw.Write(data)
				require.Equal(t, http.StatusOK, w.Code)
				require.DeepEqual(t, data, w.Body.Bytes())
			},
		},
		{
			name: "with callback",
			run: func(t *testing.T) {
				w := httptest.NewRecorder()
				rw := newResponseWriter(ctx, w, &mockHandler{})

				data := []byte("test data")
				rw.Write(data)
				require.Equal(t, http.StatusOK, w.Code)
				require.DeepEqual(t, data, w.Body.Bytes())
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, c.run)
	}
}

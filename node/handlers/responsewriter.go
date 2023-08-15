package handlers

import (
	"context"
	"net/http"
	gohttp "net/http"
)

var _ gohttp.ResponseWriter = (*responseWriter)(nil)

type responseHandler interface {
	status(statusCode int)
	update(context.Context, []byte) ([]byte, error)
}

// responseWriter intercepts response written by upstream handler.
// number of bytes written to it.
type responseWriter struct {
	ctx           context.Context
	w             http.ResponseWriter
	h             responseHandler
	headerWritten bool
}

func newResponseWriter(
	ctx context.Context,
	w http.ResponseWriter,
	h responseHandler,
) *responseWriter {
	return &responseWriter{
		ctx: ctx,
		w:   w,
		h:   h,
	}
}

func (w *responseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *responseWriter) Write(data []byte) (int, error) {
	if !w.headerWritten {
		if w.h != nil {
			w.h.status(gohttp.StatusOK)
		}
		w.w.WriteHeader(gohttp.StatusOK)
		w.headerWritten = true
	}

	if w.h != nil {
		var err error
		if data, err = w.h.update(w.ctx, data); err != nil {
			return 0, err
		}
	}

	if len(data) == 0 {
		return 0, nil
	}

	return w.w.Write(data)
}

func (w *responseWriter) WriteHeader(statusCode int) {
	if w.headerWritten {
		return
	}

	if w.h != nil {
		w.h.status(statusCode)
	}
	w.w.WriteHeader(gohttp.StatusOK)
	w.headerWritten = true
}

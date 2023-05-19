package node

import (
	"errors"
	"io"
	"net/http"
)

var (
	ErrSizeCapExceeded = errors.New("body size cap exceeded")
)

// ingressCounter is a wrapper for the underlying http.Request.Body and counts
// number of bytes consumed from it.
type ingressCounter struct {
	body    io.ReadCloser
	maxSize int
	sz      int
}

func newIngressCounter(body io.ReadCloser, maxSize int) *ingressCounter {
	return &ingressCounter{
		body:    body,
		maxSize: maxSize,
		sz:      0,
	}
}

func (c *ingressCounter) Read(p []byte) (int, error) {
	n, err := c.body.Read(p)
	c.sz += n
	if c.maxSize > 0 && c.sz > c.maxSize {
		return 0, ErrSizeCapExceeded
	}
	return n, err
}

func (c *ingressCounter) Close() error {
	return c.body.Close()
}

func (c *ingressCounter) size() int {
	return c.sz
}

// egressCounter is a wrapper for the underlying http.ResponseWriter and counts
// number of bytes written to it.
type egressCounter struct {
	w  http.ResponseWriter
	sz int
}

func newEgressCounter(w http.ResponseWriter) *egressCounter {
	return &egressCounter{
		w:  w,
		sz: 0,
	}
}

func (c *egressCounter) Header() http.Header {
	return c.w.Header()
}

func (c *egressCounter) Write(data []byte) (int, error) {
	c.sz += len(data)
	return c.w.Write(data)
}

func (c *egressCounter) WriteHeader(statusCode int) {
	c.w.WriteHeader(statusCode)
}

func (c *egressCounter) size() int {
	return c.sz
}

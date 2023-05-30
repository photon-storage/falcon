package node

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"

	"github.com/photon-storage/go-common/metrics"
)

var (
	ErrBadContent = errors.New("bad content")
)

// contentSentry is a wrapper for the underlying http.ResponseWriter and
// checks & blocks for potential malicious content.
type contentSentry struct {
	w       http.ResponseWriter
	flagged bool
}

func newContentSentry(w http.ResponseWriter) *contentSentry {
	return &contentSentry{
		w: w,
	}
}

func (s *contentSentry) Header() http.Header {
	return s.w.Header()
}

func (s *contentSentry) Write(data []byte) (int, error) {
	if s.flagged {
		return 0, ErrBadContent
	}

	for _, rl := range rules {
		if bytes.Contains(data, rl.exact) {
			s.flagged = true
			metrics.CounterInc(fmt.Sprintf(
				"request_blocked_total.rule#%v",
				rl.name,
			))
			return 0, ErrBadContent
		}
	}

	return s.w.Write(data)
}

func (s *contentSentry) WriteHeader(statusCode int) {
	s.w.WriteHeader(statusCode)
}

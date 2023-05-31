package node

import (
	"bytes"
	"errors"
	"net/http"
	"strings"
)

var (
	ErrBadContent = errors.New("bad content")
)

const (
	batchSize = 64 * 1024
)

// contentSentry is a wrapper for the underlying http.ResponseWriter and
// checks & blocks for potential malicious content.
type contentSentry struct {
	w               http.ResponseWriter
	enableDetection bool
	statusCode      int
	headerWritten   bool
	buf             []byte
	flaggedRuleName string
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
	// No detection, shortcut.
	if !s.enableDetection {
		return s.w.Write(data)
	}

	// Previous flush signals a bad content, return immediately.
	if s.flaggedRuleName != "" {
		return 0, ErrBadContent
	}

	// Accumulate batch and flush if needed.
	s.buf = append(s.buf, data...)
	if len(s.buf) >= batchSize {
		if err := s.flush(); err != nil {
			return 0, err
		}
	}

	// If the flush detects a bad content, returns error.
	if s.flaggedRuleName != "" {
		return 0, ErrBadContent
	}

	return len(data), nil
}

// WriteHeader is called before actual data written. Check if sentry check
// should be enabled based on content type. If check is enabled, delay sending
// header and status code so we could send http.StatusGone when bad content
// is found.
func (s *contentSentry) WriteHeader(statusCode int) {
	ctype := s.w.Header().Get("Content-Type")
	s.enableDetection = shouldEnableDetection(ctype)
	s.statusCode = statusCode
	if !s.enableDetection {
		s.w.WriteHeader(s.statusCode)
		s.headerWritten = true
	}
}

func (s *contentSentry) flush() error {
	// Empty batch buffer. Either no detection is enabled or no accumulation
	// happened since the last flush.
	if len(s.buf) == 0 {
		return nil
	}

	// Check buf data if no flag was raised previously.
	if s.flaggedRuleName == "" {
		s.flaggedRuleName = checkSentryRule(s.buf)
	}

	if s.flaggedRuleName == "" {
		// The batch is good. Send header if needed and flush the data.
		if !s.headerWritten {
			s.w.WriteHeader(s.statusCode)
			s.headerWritten = true
		}
		_, err := s.w.Write(s.buf)
		s.buf = s.buf[:0]
		return err
	} else {
		// Bad batch detected. Sending http.StatusGone without content.
		if !s.headerWritten {
			header := s.w.Header()
			delete(header, "Content-Length")
			delete(header, "Content-Type")
			delete(header, "Etag")
			delete(header, "X-Ipfs-Path")
			delete(header, "X-Ipfs-Roots")
			s.w.WriteHeader(http.StatusGone)
			s.headerWritten = true
		}
	}

	return nil
}

func (s *contentSentry) getFlaggedRuleName() string {
	return s.flaggedRuleName
}

func shouldEnableDetection(ctype string) bool {
	if ctype == "" {
		return true
	}
	if strings.HasPrefix(ctype, "text/html") {
		return true
	}
	return false
}

func checkSentryRule(data []byte) string {
	for _, rl := range rules {
		if bytes.Contains(data, rl.exact) {
			return rl.name
		}
	}
	return ""
}

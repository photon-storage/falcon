package node

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"
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
	mu              sync.Mutex
}

func newContentSentry(
	ctx context.Context,
	w http.ResponseWriter,
) *contentSentry {
	s := &contentSentry{
		w: w,
	}

	ticker := time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
				s.mu.Lock()
				if !s.headerWritten {
					s.w.WriteHeader(http.StatusProcessing)
				} else {
					s.mu.Unlock()
					return
				}
				s.mu.Unlock()
			}
		}
	}()

	return s
}

func (s *contentSentry) Header() http.Header {
	return s.w.Header()
}

func (s *contentSentry) Write(data []byte) (int, error) {
	// No detection, shortcut.
	if !s.enableDetection {
		s.buf = data
		if err := s.flush(); err != nil {
			return 0, err
		}
		return len(data), nil
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
		s.buf = s.buf[:0]
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
}

func (s *contentSentry) flush() error {
	// Empty batch buffer. Either no detection is enabled or no accumulation
	// happened since the last flush.
	if len(s.buf) == 0 {
		return nil
	}

	// Check buf data if no flag was raised previously.
	if s.enableDetection && s.flaggedRuleName == "" {
		s.flaggedRuleName = checkSentryRule(s.buf)
	}

	if s.flaggedRuleName == "" {
		// The batch is good. Send header if needed and flush the data.
		s.mu.Lock()
		if !s.headerWritten {
			s.w.WriteHeader(s.statusCode)
			s.headerWritten = true
		}
		s.mu.Unlock()
		_, err := s.w.Write(s.buf)
		return err
	} else {
		// Bad batch detected. Sending http.StatusGone without content.
		s.mu.Lock()
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
		s.mu.Unlock()
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

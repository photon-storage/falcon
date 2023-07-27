package node

import "time"

var (
	uriTimeouts = map[string]time.Duration{
		"/api/v0/pin/add": 3600 * time.Second,
	}
	defaultUriTimeout = 600 * time.Second
)

func getUriTimeout(uri string) time.Duration {
	to, ok := uriTimeouts[uri]
	if ok {
		return to
	}

	return defaultUriTimeout
}

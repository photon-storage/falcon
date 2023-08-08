package handlers

import (
	gohttp "net/http"
)

func (h *ExtendedHandlers) DagImport() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		// TODO(kmax): intercept response and handle pin success reporting
		h.apiHandlers.ServeHTTP(w, r)
	})
}

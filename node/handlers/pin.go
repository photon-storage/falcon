package handlers

import (
	"fmt"
	gohttp "net/http"

	"github.com/ipfs/go-cid"

	"github.com/photon-storage/go-gw3/common/http"

	"github.com/photon-storage/falcon/node/com"
)

type PinnedCountResult struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}

func (h *ExtendedHandlers) PinnedCount() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		query := r.URL.Query()
		c, err := cid.Decode(query.Get(http.ParamIPFSArg))
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinnedCountResult{
					Count:   -1,
					Message: "invalid CID",
				},
			)
			return
		}

		pinner := com.GetRcPinner(h.nd.Pinning)
		if pinner == nil {
			writeJSON(
				w,
				gohttp.StatusNotImplemented,
				&PinnedCountResult{
					Count:   -1,
					Message: "pinner does not support pinned count query",
				},
			)
			return
		}

		count, err := pinner.PinnedCount(r.Context(), c)
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusInternalServerError,
				&PinnedCountResult{
					Count:   -1,
					Message: fmt.Sprintf("error querying count: %v", err),
				},
			)
			return
		}

		writeJSON(
			w,
			gohttp.StatusOK,
			&PinnedCountResult{
				Count:   int(count),
				Message: "ok",
			},
		)
	})
}

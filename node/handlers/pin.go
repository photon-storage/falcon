package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	gohttp "net/http"
	"strings"

	coreiface "github.com/ipfs/boxo/coreiface"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/core/commands/pin"

	"github.com/photon-storage/go-gw3/common/http"

	"github.com/photon-storage/falcon/node/com"
)

var (
	ErrInvalidCID           = errors.New("invalid CID")
	ErrInvalidRecursiveFlag = errors.New("invalid value for recursive flag")
)

type pinAddRespHandler struct {
	statusCode int
	api        coreiface.CoreAPI
	root       cid.Cid
	recursive  bool
	dagStats   *DagStats
}

func (h *pinAddRespHandler) status(statusCode int) {
	if h.statusCode != 0 {
		h.statusCode = statusCode
	}
}

func (h *pinAddRespHandler) update(
	ctx context.Context,
	data []byte,
) ([]byte, error) {
	// Only convert responses that we understand.
	var val pin.AddPinOutput
	if err := json.Unmarshal(data, &val); err == nil {
		if len(val.Pins) == 0 {
			return json.Marshal(&PinAddResult{
				Done:               false,
				ProcessedNumBlocks: val.Progress,
			})
		}

		ds := h.dagStats
		if ds == nil {
			ds = NewDagStats()
		}

		// Completed.
		if err := CalculateDagStats(
			ctx,
			h.api,
			h.root,
			h.recursive,
			ds,
		); err != nil {
			return nil, err
		}

		return json.Marshal(&PinAddResult{
			Done:                  true,
			ProcessedNumBlocks:    val.Progress,
			DeduplicatedSize:      ds.DeduplicatedSize.Load(),
			DeduplicatedNumBlocks: ds.DeduplicatedNumBlocks.Load(),
			TotalSize:             ds.TotalSize.Load(),
			TotalNumBlocks:        ds.TotalNumBlocks.Load(),
		})
	}

	return data, nil
}

type PinAddResult struct {
	Done                  bool   `json:"done,omitempty"`
	ProcessedNumBlocks    int    `json:"processed_num_blocks,omitempty"`
	DeduplicatedSize      int64  `json:"duplicated_size,omitempty"`
	DeduplicatedNumBlocks int64  `json:"duplicated_num_blocks,omitempty"`
	TotalSize             int64  `json:"total_size,omitempty"`
	TotalNumBlocks        int64  `json:"total_num_blocks,omitempty"`
	Message               string `json:"message"`
}

func (h *ExtendedHandlers) PinAdd() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		c, recursive, err := parsePinParams(r)
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinAddResult{
					Message: err.Error(),
				},
			)
			return
		}

		h.apiHandlers.ServeHTTP(
			newResponseWriter(
				r.Context(),
				w,
				&pinAddRespHandler{
					api:       h.api,
					root:      c,
					recursive: recursive,
					dagStats:  getDagStatsFromCtx(r.Context()),
				},
			),
			r,
		)
	})
}

type PinnedCountResult struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}

func (h *ExtendedHandlers) PinnedCount() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		c, recursive, err := parsePinParams(r)
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinnedCountResult{
					Count:   -1,
					Message: err.Error(),
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

		count, err := pinner.PinnedCount(r.Context(), c, recursive)
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

func parsePinParams(r *gohttp.Request) (cid.Cid, bool, error) {
	query := r.URL.Query()
	c, err := cid.Decode(query.Get(http.ParamIPFSArg))
	if err != nil {
		return cid.Undef, false, ErrInvalidCID
	}

	recursiveStr := strings.ToLower(query.Get(http.ParamIPFSRecursive))
	recursive := false
	if recursiveStr == "1" || recursiveStr == "true" {
		recursive = true
	} else if recursiveStr == "0" || recursiveStr == "false" {
		recursive = false
	} else {
		return cid.Undef, false, ErrInvalidRecursiveFlag
	}

	return c, recursive, nil
}

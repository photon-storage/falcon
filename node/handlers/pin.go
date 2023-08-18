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
	rcpinner "github.com/photon-storage/go-rc-pinner"

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
				Success:            false,
				InProgress:         true,
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
			return json.Marshal(&PinAddResult{
				Success:            false,
				InProgress:         false,
				ProcessedNumBlocks: val.Progress,
				Message:            fmt.Sprintf("dag stats error: %v", err),
			})
		}

		return json.Marshal(&PinAddResult{
			Success:               true,
			InProgress:            false,
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
	Success               bool   `json:"success"`
	InProgress            bool   `json:"in_progress"`
	ProcessedNumBlocks    int    `json:"processed_num_blocks"`
	DeduplicatedSize      int64  `json:"duplicated_size"`
	DeduplicatedNumBlocks int64  `json:"duplicated_num_blocks"`
	TotalSize             int64  `json:"total_size"`
	TotalNumBlocks        int64  `json:"total_num_blocks"`
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
					Success:    false,
					InProgress: false,
					Message:    fmt.Sprintf("error parsing params: %v", err),
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

type pinRmRespHandler struct {
	statusCode int
	api        coreiface.CoreAPI
	root       cid.Cid
	recursive  bool
	dagStats   *DagStats
}

func (h *pinRmRespHandler) status(statusCode int) {
	if h.statusCode != 0 {
		h.statusCode = statusCode
	}
}

func (h *pinRmRespHandler) update(
	ctx context.Context,
	data []byte,
) ([]byte, error) {
	// Only convert responses that we understand.
	var val pin.PinOutput
	if err := json.Unmarshal(data, &val); err == nil {
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
			return json.Marshal(&PinRmResult{
				Success: false,
				Message: fmt.Sprintf("dag stats error: %v", err),
			})
		}

		return json.Marshal(&PinRmResult{
			Success:               true,
			DeduplicatedSize:      ds.DeduplicatedSize.Load(),
			DeduplicatedNumBlocks: ds.DeduplicatedNumBlocks.Load(),
			TotalSize:             ds.TotalSize.Load(),
			TotalNumBlocks:        ds.TotalNumBlocks.Load(),
		})
	}

	return data, nil
}

type PinRmResult struct {
	Success               bool   `json:"success"`
	DeduplicatedSize      int64  `json:"duplicated_size"`
	DeduplicatedNumBlocks int64  `json:"duplicated_num_blocks"`
	TotalSize             int64  `json:"total_size"`
	TotalNumBlocks        int64  `json:"total_num_blocks"`
	Message               string `json:"message"`
}

func (h *ExtendedHandlers) PinRm() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		c, recursive, err := parsePinParams(r)
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinRmResult{
					Success: false,
					Message: fmt.Sprintf("error parsing params: %v", err),
				},
			)
			return
		}

		h.apiHandlers.ServeHTTP(
			newResponseWriter(
				r.Context(),
				w,
				&pinRmRespHandler{
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

type CidCount struct {
	Cid   string `json:"c"`
	Count int    `json:"v"`
}

type PinListResult struct {
	Success    bool        `json:"success"`
	InProgress bool        `json:"in_progress"`
	Batch      []*CidCount `json:"batch"`
	Message    string      `json:"message"`
}

const cidBatchSize = 100

func (h *ExtendedHandlers) PinList() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		recursive, err := parseRecursiveParam(r)
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinListResult{
					Success:    false,
					InProgress: false,
					Message:    fmt.Sprintf("error parsing params: %v", err),
				},
			)
			return
		}

		pinner := com.GetRcPinner(h.nd.Pinning)
		if pinner == nil {
			writeJSON(
				w,
				gohttp.StatusNotImplemented,
				&PinListResult{
					Success:    false,
					InProgress: false,
					Message:    "pinner does not support pinned count query",
				},
			)
			return
		}

		var ch <-chan *rcpinner.StreamedCidWithCount
		if recursive {
			ch = pinner.RecursiveKeysWithCount(r.Context())
		} else {
			ch = pinner.DirectKeysWithCount(r.Context())
		}

		var batch []*CidCount
		for v := range ch {
			if v.Cid.Err != nil {
				writeJSON(
					w,
					gohttp.StatusNotImplemented,
					&PinListResult{
						Success:    false,
						InProgress: false,
						Message:    fmt.Sprintf("pinner index error: %v", v.Cid.Err),
					},
				)
				return
			}

			batch = append(batch, &CidCount{
				Cid:   v.Cid.C.String(),
				Count: int(v.Count),
			})

			if len(batch) >= cidBatchSize {
				data, _ := json.Marshal(&PinListResult{
					Success:    false,
					InProgress: true,
					Batch:      batch,
				})
				w.Write(data)
				batch = nil
			}
		}

		data, _ := json.Marshal(&PinListResult{
			Success:    true,
			InProgress: false,
			Batch:      batch,
		})
		w.Write(data)
	})
}

type PinnedCountResult struct {
	Success bool   `json:"success"`
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
					Success: false,
					Count:   0,
					Message: fmt.Sprintf("error parsing params: %v", err),
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
					Success: false,
					Count:   0,
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
					Success: false,
					Count:   0,
					Message: fmt.Sprintf("error querying count: %v", err),
				},
			)
			return
		}

		writeJSON(
			w,
			gohttp.StatusOK,
			&PinnedCountResult{
				Success: true,
				Count:   int(count),
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

	recursive, err := parseRecursiveParam(r)
	if err != nil {
		return cid.Undef, false, err
	}

	return c, recursive, nil
}

func parseRecursiveParam(r *gohttp.Request) (bool, error) {
	query := r.URL.Query()
	recursiveStr := strings.ToLower(query.Get(http.ParamIPFSRecursive))
	if recursiveStr == "1" || recursiveStr == "true" {
		return true, nil
	} else if recursiveStr == "0" || recursiveStr == "false" {
		return false, nil
	}

	return false, ErrInvalidRecursiveFlag
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	gohttp "net/http"
	"strings"

	coreiface "github.com/ipfs/boxo/coreiface"
	coreifacepath "github.com/ipfs/boxo/coreiface/path"
	"github.com/ipfs/boxo/ipld/merkledag"
	pinneriface "github.com/ipfs/boxo/pinning/pinner"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/kubo/core/commands/pin"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-gw3/common/http"
	rcpinner "github.com/photon-storage/go-rc-pinner"

	"github.com/photon-storage/falcon/node/com"
)

var (
	ErrInvalidCID           = errors.New("invalid CID")
	ErrInvalidRecursiveFlag = errors.New("invalid value for recursive flag")
	ErrCIDNotChild          = errors.New("CID is not a child from root")
	ErrCIDDuplicated        = errors.New("duplicated CID found")
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

		ds := NewDagStats()
		if err := CalculateDagStats(
			ctx,
			h.api,
			h.root,
			h.recursive,
			ds,
		); err != nil {
			// Ignore stats error.
			log.Error("Error calculating dag stats",
				"error", err,
				"cid", h.root.String(),
				"source", "pin add",
			)
		}
		if h.dagStats != nil {
			h.dagStats.Add(ds)
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
		ds := NewDagStats()
		if err := CalculateDagStats(
			ctx,
			h.api,
			h.root,
			h.recursive,
			ds,
		); err != nil {
			// Ignore stats error.
			log.Error("Error calculating dag stats",
				"error", err,
				"cid", h.root.String(),
				"source", "pin rm",
			)
		}
		if h.dagStats != nil {
			h.dagStats.Sub(ds)
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

type PinChildrenUpdate struct {
	Cid       string `json:"c"`
	Recursive bool   `json:"r"`
}

type PinChildrenUpdateRequest struct {
	Root string               `json:"root"`
	Incs []*PinChildrenUpdate `json:"incs"`
	Decs []*PinChildrenUpdate `json:"decs"`
}

type PinChildrenUpdateResult struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Sizes   map[string]uint64 `json:"sizes"`
}

// PinChildrenUpdate updates a root node's children's reference count in
// pinner index. The API requires the root node is already pinned recursively.
// All updating CIDs must be current child of the root node. For decrementing
// CID, its current count must be positive, otherwise, the all updates abort.
func (h *ExtendedHandlers) PinChildrenUpdate() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		var data []byte
		if r.Body != nil {
			data, _ = io.ReadAll(r.Body)
		}
		var pcu PinChildrenUpdateRequest
		if err := json.Unmarshal(data, &pcu); err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinChildrenUpdateResult{
					Success: false,
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
				&PinChildrenUpdateResult{
					Success: false,
					Message: "pinner does not support pinned count query",
				},
			)
			return
		}

		incs, decs, err := validateChildrenUpdate(
			r.Context(),
			h.api,
			pinner,
			&pcu,
		)
		if err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinChildrenUpdateResult{
					Success: false,
					Message: fmt.Sprintf("invalid request: %v", err),
				},
			)
			return
		}

		if err := pinner.UpdateCounts(r.Context(), incs, decs); err != nil {
			writeJSON(
				w,
				gohttp.StatusBadRequest,
				&PinChildrenUpdateResult{
					Success: false,
					Message: fmt.Sprintf("error updating counts: %v", err),
				},
			)
			return
		}

		aggrDs := getDagStatsFromCtx(r.Context())
		sizes := map[string]uint64{}
		for idx, updates := range [][]*rcpinner.UpdateCount{incs, decs} {
			for _, u := range updates {
				ds := NewDagStats()
				if err := CalculateDagStats(
					r.Context(),
					h.api,
					u.CID,
					u.Recursive,
					ds,
				); err != nil {
					// Ignore stats error.
					log.Error("Error calculating dag stats",
						"error", err,
						"cid", u.CID.String(),
						"source", "pin children update",
					)
				}

				sizes[u.CID.String()] = uint64(ds.TotalSize.Load())
				if aggrDs != nil {
					if idx == 0 {
						aggrDs.Add(ds)
					} else {
						aggrDs.Sub(ds)
					}
				}
			}
		}

		writeJSON(
			w,
			gohttp.StatusOK,
			&PinChildrenUpdateResult{
				Success: true,
				Message: "ok",
				Sizes:   sizes,
			},
		)
	})
}

func validateChildrenUpdate(
	ctx context.Context,
	api coreiface.CoreAPI,
	pinner *com.WrappedPinner,
	r *PinChildrenUpdateRequest,
) ([]*rcpinner.UpdateCount, []*rcpinner.UpdateCount, error) {
	rootCid, err := cid.Parse(r.Root)
	if err != nil {
		return nil, nil, err
	}
	cnt, err := pinner.GetCount(ctx, rootCid, true)
	if err != nil {
		return nil, nil, err
	}
	if cnt == 0 {
		return nil, nil, pinneriface.ErrNotPinned
	}

	br, err := api.Block().Get(ctx, coreifacepath.New(r.Root))
	if err != nil {
		return nil, nil, err
	}

	data, err := io.ReadAll(br)
	if err != nil {
		return nil, nil, err
	}
	rootNd, err := merkledag.DecodeProtobuf(data)
	if err != nil {
		return nil, nil, err
	}

	m := map[string]bool{}
	for _, link := range rootNd.Links() {
		m[link.Cid.String()] = true
	}
	seen := map[string]bool{}

	dupKey := func(c string, r bool) string {
		return fmt.Sprintf("%v_%v", c, r)
	}
	check := func(u *PinChildrenUpdate, set bool) (*rcpinner.UpdateCount, error) {
		if !m[u.Cid] {
			return nil, ErrCIDNotChild
		}

		dk := dupKey(u.Cid, u.Recursive)
		if set {
			seen[dk] = true
		} else if seen[dk] {
			return nil, ErrCIDDuplicated
		}

		k, err := cid.Parse(u.Cid)
		if err != nil {
			return nil, ErrInvalidCID
		}

		return &rcpinner.UpdateCount{
			CID:       k,
			Recursive: u.Recursive,
		}, nil
	}

	var incs []*rcpinner.UpdateCount
	for _, u := range r.Incs {
		u, err := check(u, true)
		if err != nil {
			return nil, nil, err
		}

		incs = append(incs, u)
	}

	var decs []*rcpinner.UpdateCount
	for _, u := range r.Decs {
		u, err := check(u, false)
		if err != nil {
			return nil, nil, err
		}

		decs = append(decs, u)
	}

	return incs, decs, nil
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
					gohttp.StatusInternalServerError,
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

		count, err := pinner.GetCount(r.Context(), c, recursive)
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

package node

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	gohttp "net/http"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipns"
	iface "github.com/ipfs/interface-go-ipfs-core"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/photon-storage/go-gw3/common/http"
)

type extendedHandlers struct {
	nd   *core.IpfsNode
	rcfg *config.Config
	api  iface.CoreAPI
}

func newExtendedHandlers(
	nd *core.IpfsNode,
	rcfg *config.Config,
	api iface.CoreAPI,
) *extendedHandlers {
	return &extendedHandlers{
		nd:   nd,
		rcfg: rcfg,
		api:  api,
	}
}

type StatusResult struct {
	Status    string `json:"status"`
	PublicKey string `json:"public_key"`
}

func (h *extendedHandlers) status() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		writeJSON(
			w,
			gohttp.StatusOK,
			&StatusResult{
				Status:    "ok",
				PublicKey: Cfg().PublicKeyBase64,
			},
		)
	})
}

type PinnedCountResult struct {
	Count   int    `json:"count"`
	Message string `json:"message"`
}

func (h *extendedHandlers) pinnedCount() gohttp.HandlerFunc {
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

		pinner := getRcPinner(h.nd.Pinning)
		if pinner == nil {
			fmt.Printf("** kmax?\n")
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

type NameBroadcastResult struct {
	Message string `json:"message"`
}

func (h *extendedHandlers) nameBroadcast() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		query := r.URL.Query()
		peerID, err := peer.Decode(query.Get(http.ParamIPFSKey))
		if err != nil {
			writeJSON(w, gohttp.StatusBadRequest, &NameBroadcastResult{
				Message: err.Error(),
			})
			return
		}

		k := ipns.RecordKey(peerID)
		v, err := base64.StdEncoding.DecodeString(query.Get(http.ParamIPFSArg))
		if err != nil {
			writeJSON(w, gohttp.StatusBadRequest, &NameBroadcastResult{
				Message: err.Error(),
			})
			return
		}

		validator := ipns.Validator{}
		if err := validator.Validate(k, v); err != nil {
			writeJSON(w, gohttp.StatusBadRequest, &NameBroadcastResult{
				Message: err.Error(),
			})
			return
		}

		if err := h.nd.Routing.PutValue(r.Context(), k, v); err != nil {
			writeJSON(w, gohttp.StatusBadRequest, &NameBroadcastResult{
				Message: err.Error(),
			})
			return
		}

		writeJSON(w, gohttp.StatusOK, &NameBroadcastResult{
			Message: "ok",
		})
	})
}

func writeJSON(w gohttp.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.Encode(v)
	if f, ok := w.(gohttp.Flusher); ok {
		f.Flush()
	}
}

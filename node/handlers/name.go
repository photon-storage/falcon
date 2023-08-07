package handlers

import (
	"encoding/base64"
	gohttp "net/http"

	"github.com/ipfs/boxo/ipns"
	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/photon-storage/go-gw3/common/http"
)

type NameBroadcastResult struct {
	Message string `json:"message"`
}

func (h *ExtendedHandlers) NameBroadcast() gohttp.HandlerFunc {
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
		v, err := base64.URLEncoding.DecodeString(query.Get(http.ParamIPFSArg))
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

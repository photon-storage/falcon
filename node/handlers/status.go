package handlers

import (
	gohttp "net/http"

	"github.com/photon-storage/falcon/node/config"
)

type StatusResult struct {
	Status    string `json:"status"`
	PublicKey string `json:"public_key"`
}

func (h *ExtendedHandlers) Status() gohttp.HandlerFunc {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		writeJSON(
			w,
			gohttp.StatusOK,
			&StatusResult{
				Status:    "ok",
				PublicKey: config.Get().PublicKeyBase64,
			},
		)
	})
}

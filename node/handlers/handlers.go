package handlers

import (
	"encoding/json"
	gohttp "net/http"

	coreiface "github.com/ipfs/boxo/coreiface"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
)

type ExtendedHandlers struct {
	nd          *core.IpfsNode
	rcfg        *config.Config
	api         coreiface.CoreAPI
	apiHandlers gohttp.Handler
}

func New(
	nd *core.IpfsNode,
	rcfg *config.Config,
	api coreiface.CoreAPI,
	apiHandlers gohttp.Handler,
) *ExtendedHandlers {
	return &ExtendedHandlers{
		nd:          nd,
		rcfg:        rcfg,
		api:         api,
		apiHandlers: apiHandlers,
	}
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

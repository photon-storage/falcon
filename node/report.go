package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	gohttp "net/http"
	"strconv"
	"time"

	coreiface "github.com/ipfs/boxo/coreiface"
	mdag "github.com/ipfs/boxo/ipld/merkledag"
	"github.com/ipfs/boxo/ipld/merkledag/traverse"
	"github.com/ipfs/go-cid"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-common/metrics"
	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/http"
	"github.com/photon-storage/go-gw3/common/reporting"
)

var (
	uriToReportCidSize = map[string]int{
		"/api/v0/pin/add": 1,
		"/api/v0/pin/rm":  -1,
	}
)

type reportHandler struct {
	coreapi coreiface.CoreAPI
}

func newReportHandler(coreapi coreiface.CoreAPI) *reportHandler {
	return &reportHandler{
		coreapi: coreapi,
	}
}

// This could be chained after auth, which decoded args.
func (h *reportHandler) wrap(next gohttp.Handler) gohttp.Handler {
	if Cfg().ExternalServices.Spaceport == "" {
		return next
	}

	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		if GetNoReportFromCtx(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}
		r = r.WithContext(SetNoReportFromCtx(r.Context()))

		maxSize := 0
		if args := GetArgsFromCtx(r.Context()); args != nil {
			if str := args.GetArg(http.ArgP3Size); str != "" {
				sz, err := strconv.ParseInt(str, 10, 64)
				if err != nil {
					gohttp.Error(
						w,
						gohttp.StatusText(gohttp.StatusBadRequest),
						gohttp.StatusBadRequest,
					)
					return
				}
				maxSize = int(sz)
			}
		}

		ingr := newIngressCounter(r.Body, maxSize)
		egr := newEgressCounter(w)
		r.Body = ingr
		w = egr

		// NOTE(kmax): Kubo handler can change URL so we need to make a
		// deep copy of the request to be safe.
		defer func(cr *gohttp.Request) {
			metrics.CounterAdd("ingress_bytes", float64(ingr.size()))
			metrics.CounterAdd("egress_bytes", float64(egr.size()))
			metrics.CounterInc("request_log_total")

			if err := reportRequest(
				cr.Context(),
				h.coreapi,
				cr,
				ingr.size(),
				egr.size(),
			); err != nil {
				metrics.CounterInc("request_log_err_total")
				log.Error("Error making log request", "error", err)
			}
		}(r.Clone(r.Context()))

		next.ServeHTTP(w, r)
	})
}

func reportRequest(
	ctx context.Context,
	coreapi coreiface.CoreAPI,
	r *gohttp.Request,
	ingr int,
	egr int,
) error {
	query := r.URL.Query()

	uri := auth.CanonicalizeURI(r.URL.Path)
	sz := 0
	if w := uriToReportCidSize[uri]; w != 0 {
		// TODO(kmax): handle error with retry, probably not gonna help?
		v := query.Get(http.ParamIPFSArg)
		if v != "" {
			k, err := cid.Decode(v)
			if err == nil {
				sz, _ = calculateCidSize(ctx, coreapi, k)
			}
		}
		sz *= w
	}

	logdata, err := json.Marshal(&reporting.LogV1{
		Version: 1,
		Req: reporting.AuthReq{
			Method: r.Method,
			URI:    uri,
			Args:   query.Get(http.ParamP3Args),
			Sig:    query.Get(http.ParamP3Sig),
		},
		CidSize: sz,
		Ingress: ingr,
		Egress:  egr,
		At:      time.Now().Unix(),
	})
	if err != nil {
		return fmt.Errorf("error marshaling log struct: %w", err)
	}

	cfg := Cfg()

	req, err := gohttp.NewRequest(
		gohttp.MethodPost,
		fmt.Sprintf(
			"%v/api/v0/put",
			cfg.ExternalServices.Spaceport,
		),
		bytes.NewReader(logdata),
	)
	if err != nil {
		return fmt.Errorf("error creating log request: %w", err)
	}

	// Set auth header
	sig, err := auth.SignBase64(logdata, cfg.SecretKey)
	if err != nil {
		return fmt.Errorf("error signing log data: %w", err)
	}
	req.Header.Set(http.HeaderAuthorization, sig)

	resp, err := cfg.HttpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error making log request: %w", err)
	}
	if resp.StatusCode != gohttp.StatusOK {
		msg, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status: [%v] %v",
			resp.StatusCode,
			string(msg),
		)
	}

	return nil
}

func calculateCidSize(
	ctx context.Context,
	coreapi coreiface.CoreAPI,
	k cid.Cid,
) (int, error) {
	nodeGetter := mdag.NewSession(ctx, coreapi.Dag())
	root, err := nodeGetter.Get(ctx, k)
	if err != nil {
		return 0, err
	}

	sz := 0
	if err := traverse.Traverse(root, traverse.Options{
		DAG:   nodeGetter,
		Order: traverse.DFSPre,
		Func: func(current traverse.State) error {
			sz += len(current.Node.RawData())
			return nil
		},
		ErrFunc:        nil,
		SkipDuplicates: true,
	}); err != nil {
		return 0, err
	}

	return sz, nil
}

package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	gohttp "net/http"
	"net/url"
	"strconv"
	"time"

	coreiface "github.com/ipfs/boxo/coreiface"
	"go.uber.org/atomic"

	"github.com/photon-storage/falcon/node/config"
	"github.com/photon-storage/falcon/node/handlers"
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

type monitorHandler struct {
	coreapi coreiface.CoreAPI
}

func newMonitorHandler(coreapi coreiface.CoreAPI) *monitorHandler {
	return &monitorHandler{
		coreapi: coreapi,
	}
}

// This should be chained after auth, which decoded args.
func (m *monitorHandler) wrap(next gohttp.Handler) gohttp.Handler {
	if config.Get().ExternalServices.Spaceport == "" {
		return next
	}

	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		if GetNoReportFromCtx(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}

		// The cancel guards reporter and the request context.
		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		ctx = SetNoReportFromCtx(ctx)
		p2pIngr := atomic.NewUint64(0)
		ctx = SetFetchSizeFromCtx(ctx, p2pIngr)
		dagStats := handlers.NewDagStats()
		ctx = SetDagStatFromCtx(ctx, dagStats)
		r = r.WithContext(ctx)

		maxSize, err := extractSizeFromArgs(r)
		if err != nil {
			gohttp.Error(
				w,
				gohttp.StatusText(gohttp.StatusBadRequest),
				gohttp.StatusBadRequest,
			)
			return
		}

		httpIngr := newIngressCounter(r.Body, maxSize)
		httpEgr := newEgressCounter(w)
		r.Body = httpIngr
		w = httpEgr

		go newMonitor(
			m.coreapi,
			r,
			httpIngr,
			httpEgr,
			p2pIngr,
			maxSize,
			dagStats,
		).run(ctx, cancel)

		next.ServeHTTP(w, r)
	})
}

type monitor struct {
	coreapi         coreiface.CoreAPI
	httpIngr        *ingressCounter
	httpEgr         *egressCounter
	p2pIngr         *atomic.Uint64
	p2pIngrReported *atomic.Uint64
	maxSize         int
	dagStats        *handlers.DagStats
	method          string
	host            string
	uri             string
	query           url.Values
}

func newMonitor(
	coreapi coreiface.CoreAPI,
	req *gohttp.Request,
	httpIngr *ingressCounter,
	httpEgr *egressCounter,
	p2pIngr *atomic.Uint64,
	maxSize int,
	dagStats *handlers.DagStats,
) *monitor {
	return &monitor{
		coreapi:         coreapi,
		httpIngr:        httpIngr,
		httpEgr:         httpEgr,
		p2pIngr:         p2pIngr,
		p2pIngrReported: atomic.NewUint64(0),
		maxSize:         maxSize,
		dagStats:        dagStats,
		method:          req.Method,
		host:            req.Host,
		uri:             auth.CanonicalizeURI(req.URL.Path),
		query:           req.URL.Query(),
	}
}

func (m *monitor) run(ctx context.Context, cancel context.CancelFunc) {
	timer := time.NewTimer(time.Second)
	done := false
	reportIncr := uint64(1) << 20 // 10 MB
	for !done {
		select {
		case <-ctx.Done():
			done = true
			break

		case <-timer.C:
			if m.maxSize > 0 && int(m.p2pIngr.Load()) > m.maxSize {
				cancel()
				done = true
				break
			}

			head := m.p2pIngr.Load()
			tail := m.p2pIngrReported.Load()
			if head-tail > reportIncr {
				if err := sendLog(
					m.method,
					m.host,
					m.uri,
					m.query,
					int(head-tail),
					0,
					0,
				); err != nil {
					metrics.CounterInc("request_log_err_total")
					log.Error("Error making in-progress log request", "error", err)
				}

				// If log fails, we under report the usage.
				m.p2pIngrReported.Store(head)
			}
		}
	}

	if err := sendLog(
		m.method,
		m.host,
		m.uri,
		m.query,
		m.httpIngr.size()+int(m.p2pIngr.Load()-m.p2pIngrReported.Load()),
		m.httpEgr.size(),
		int(m.dagStats.TotalSize.Load())*uriToReportCidSize[m.uri],
	); err != nil {
		metrics.CounterInc("request_log_err_total")
		log.Error("Error making log request", "error", err)
	}
}

func sendLog(
	method string,
	host string,
	uri string,
	query url.Values,
	ingr int,
	egr int,
	pinnedBytes int,
) error {
	metrics.CounterAdd("ingress_bytes", float64(ingr))
	metrics.CounterAdd("egress_bytes", float64(egr))
	metrics.CounterInc("request_log_total")

	logData, err := json.Marshal(&reporting.LogV1{
		Version: 1,
		Req: reporting.AuthReq{
			Method: method,
			Host:   host,
			URI:    uri,
			Args:   query.Get(http.ParamP3Args),
			Sig:    query.Get(http.ParamP3Sig),
		},
		CidSize: pinnedBytes,
		Ingress: ingr,
		Egress:  egr,
		At:      time.Now().Unix(),
	})
	if err != nil {
		return fmt.Errorf("error marshaling log struct: %w", err)
	}

	cfg := config.Get()

	req, err := gohttp.NewRequest(
		gohttp.MethodPost,
		fmt.Sprintf(
			"%v/api/v0/put",
			cfg.ExternalServices.Spaceport,
		),
		bytes.NewReader(logData),
	)
	if err != nil {
		return fmt.Errorf("error creating log request: %w", err)
	}

	// Set auth header
	sig, err := auth.SignBase64(logData, cfg.SecretKey)
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

func extractSizeFromArgs(r *gohttp.Request) (int, error) {
	size := 0
	if args := GetArgsFromCtx(r.Context()); args != nil {
		if str := args.GetArg(http.ArgP3Size); str != "" {
			sz, err := strconv.ParseInt(str, 10, 64)
			if err != nil {
				return 0, nil
			}
			size = int(sz)
		}
	}
	return size, nil
}

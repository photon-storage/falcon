package node

import (
	"fmt"
	"hash/fnv"
	gohttp "net/http"
	"net/url"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-common/metrics"
	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/http"
)

type authHandler struct {
	pk                libp2pcrypto.PubKey
	redirectOnFailure bool
	starbaseURL       *url.URL
	gws               *hostnameGateways
	wl                map[string]bool
	recentSeen        *lru.Cache[uint64, bool]
}

func newAuthHandler(gws *hostnameGateways) (*authHandler, error) {
	cfg := Cfg()
	var pk libp2pcrypto.PubKey
	var starbaseURL *url.URL
	if cfg.Auth.NoAuth {
		log.Warn("Falcon API authentication is disabled")
	} else {
		var err error
		if pk, err = auth.DecodePk(
			cfg.Auth.StarbasePublicKeyBase64,
		); err != nil {
			return nil, err
		}

		dst := Cfg().ExternalServices.Starbase
		if !strings.HasPrefix(dst, "http://") &&
			!strings.HasPrefix(dst, "https://") {
			dst = "http://" + dst
		}
		if starbaseURL, err = url.Parse(dst); err != nil {
			return nil, err
		}
	}

	wl := map[string]bool{}
	for _, ns := range cfg.Auth.Whitelist {
		wl[ns] = true
	}

	// Assuming QPS = 10k
	cache, err := lru.New[uint64, bool](1024 * 600)
	if err != nil {
		return nil, err
	}

	return &authHandler{
		pk:                pk,
		redirectOnFailure: cfg.Auth.RedirectOnFailure,
		starbaseURL:       starbaseURL,
		gws:               gws,
		wl:                wl,
		recentSeen:        cache,
	}, nil
}

func (h *authHandler) hasRecentSeen(r *gohttp.Request) bool {
	s := fnv.New64a()
	s.Write([]byte(r.URL.String()))
	found, _ := h.recentSeen.ContainsOrAdd(s.Sum64(), true)
	return found
}

func (h *authHandler) wrap(next gohttp.Handler) gohttp.Handler {
	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		if GetNoAuthFromCtx(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}

		sw := newContentSentry(w)
		defer func() {
			sw.flush()

			flagged := sw.getFlaggedRuleName()
			if flagged != "" {
				metrics.CounterInc(fmt.Sprintf(
					"request_blocked_total.rule#%v",
					flagged,
				))
			} else {
				metrics.CounterInc("request_served_total")
			}
		}()

		if false && r.Method != gohttp.MethodOptions && h.hasRecentSeen(r) {
			gohttp.Error(
				sw,
				gohttp.StatusText(gohttp.StatusUnauthorized),
				gohttp.StatusUnauthorized,
			)
			return
		}

		wlPath := auth.CanonicalizeURI(r.URL.Path)
		if strings.HasPrefix(wlPath, "/ipfs/") {
			wlPath = "/ipfs"
		} else if strings.HasPrefix(wlPath, "/ipns/") {
			wlPath = "/ipns"
		} else if !strings.HasPrefix(wlPath, "/api/v0/") {
			// Check subdomain namespace
			host := r.Host
			if xHost := r.Header.Get("X-Forwarded-Host"); xHost != "" {
				host = xHost
			}
			if _, _, ns, _, ok := h.gws.knownSubdomainDetails(host); ok {
				if ns == "ipfs" {
					wlPath = "/ipfs"
				} else if ns == "ipns" {
					wlPath = "/ipns"
				}
			}
		}

		if !h.wl[wlPath] {
			gohttp.Error(
				sw,
				gohttp.StatusText(gohttp.StatusNotFound),
				gohttp.StatusNotFound,
			)
			return
		}
		if r.Method != gohttp.MethodOptions && h.pk != nil {
			if err := auth.VerifyRequest(r, h.pk); err != nil {
				if err == auth.ErrReqSigMissing && h.redirectOnFailure {
					redirectToStarbase(sw, r, h.starbaseURL)
				} else {
					log.Debug("Authentication failure", "error", err)
					gohttp.Error(
						sw,
						gohttp.StatusText(gohttp.StatusUnauthorized),
						gohttp.StatusUnauthorized,
					)
				}
				return
			}
		}

		r = r.WithContext(SetNoAuthFromCtx(r.Context()))

		// Reset query params and headers to trim unexpected params and
		// headers from requests. This ensures all params and headers
		// are guarded by Starbase signature.
		orig := r.URL.Query()
		query := url.Values{}
		query.Set(http.ParamP3Args, orig.Get(http.ParamP3Args))
		query.Set(http.ParamP3Sig, orig.Get(http.ParamP3Sig))
		r.URL.RawQuery = query.Encode()
		r.Header = gohttp.Header{}

		// Recover query parameters and headers from P3 args.
		p3args := query.Get(http.ParamP3Args)
		if p3args != "" {
			args, err := http.DecodeArgs(p3args)
			if err != nil {
				log.Debug("Error decoding P3 args", "error", err)
				gohttp.Error(
					sw,
					gohttp.StatusText(gohttp.StatusBadRequest),
					gohttp.StatusBadRequest,
				)
				return
			}

			if _, err := auth.ValidateTimestamp(
				args,
				10*time.Minute,
			); err != nil {
				gohttp.Error(
					sw,
					gohttp.StatusText(gohttp.StatusBadRequest),
					gohttp.StatusBadRequest,
				)
				return
			}

			for k, v := range args.Params {
				query.Set(k, v)
			}
			r.URL.RawQuery = query.Encode()

			for k, v := range args.Headers {
				r.Header.Set(k, v)
			}

			r = r.WithContext(SetArgsFromCtx(r.Context(), args))
		}

		next.ServeHTTP(sw, r)
	})
}

func redirectToStarbase(
	w gohttp.ResponseWriter,
	r *gohttp.Request,
	target *url.URL,
) {
	url := *r.URL
	url.Scheme = target.Scheme
	url.Host = target.Host
	gohttp.Redirect(w, r, url.String(), gohttp.StatusTemporaryRedirect)
}

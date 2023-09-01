package node

import (
	"context"
	"hash/fnv"
	gohttp "net/http"
	"net/textproto"
	"net/url"
	"strings"
	"time"

	lru "github.com/hashicorp/golang-lru/v2"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-common/metrics"
	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/http"

	"github.com/photon-storage/falcon/node/config"
)

var (
	headerWhitelist = []string{
		textproto.CanonicalMIMEHeaderKey("Content-Type"),
	}
)

type authHandler struct {
	pk                libp2pcrypto.PubKey
	redirectOnFailure bool
	gws               *hostnameGateways
	wl                map[string]bool
	recentSeen        *lru.Cache[uint64, bool]
}

func newAuthHandler(gws *hostnameGateways) (*authHandler, error) {
	cfg := config.Get()
	var pk libp2pcrypto.PubKey
	if cfg.Auth.NoAuth {
		log.Warn("Falcon API authentication is disabled")
	} else {
		var err error
		if pk, err = auth.DecodePk(
			cfg.Auth.StarbasePublicKeyBase64,
		); err != nil {
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

		uri := auth.CanonicalizeURI(r.URL.Path)

		// Set global request timeout.
		ctx, cancel := context.WithTimeout(r.Context(), getUriTimeout(uri))
		defer cancel()
		r = r.WithContext(ctx)

		metrics.CounterInc("request_call_total")
		// Disable sentry
		/*
			sw := newContentSentry(ctx, w)
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
		*/
		sw := w

		if false && r.Method != gohttp.MethodOptions && h.hasRecentSeen(r) {
			gohttp.Error(
				sw,
				gohttp.StatusText(gohttp.StatusUnauthorized),
				gohttp.StatusUnauthorized,
			)
			return
		}

		extractAccessCode(r)

		if strings.HasPrefix(uri, "/ipfs/") {
			uri = "/ipfs"
		} else if strings.HasPrefix(uri, "/ipns/") {
			uri = "/ipns"
		} else if !strings.HasPrefix(uri, "/api/v0/") {
			// Check subdomain namespace
			// In our case, always honor the real hostname.
			// The http.HeaderForwaredHost header should only be extracted
			// from Args, which is used by starbase to control if subdomain
			// is enabled.
			if _, _, ns, _, ok := h.gws.knownSubdomainDetails(r.Host); ok {
				if ns == "ipfs" {
					uri = "/ipfs"
				} else if ns == "ipns" {
					uri = "/ipns"
				}
			}
		}

		if !h.wl[uri] {
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
					redirectToStarbase(sw, r)
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

		r = r.WithContext(WithNoAuth(r.Context()))

		// Reset query params and headers to trim unexpected params and
		// headers from requests. This ensures all params and headers
		// are guarded by Starbase signature.
		orig := r.URL.Query()
		query := url.Values{}
		if h.pk == nil &&
			(strings.HasPrefix(r.Host, "localhost") ||
				strings.HasPrefix(r.Host, "127.0.0.1")) {
			// Use original query parameters if this is local run with
			// auth disabled.
			// NOTE: r.URL.Host can be empty.
			query = orig
		} else {
			query.Set(http.ParamP3Args, orig.Get(http.ParamP3Args))
			query.Set(http.ParamP3Sig, orig.Get(http.ParamP3Sig))
		}

		// Keep whitelisted headers
		header := gohttp.Header{}
		for _, k := range headerWhitelist {
			if v := r.Header.Values(k); v != nil {
				header[k] = v
			}
		}
		r.Header = header

		// Recover query parameters and headers from P3 args.
		p3args := orig.Get(http.ParamP3Args)
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
				parts := strings.Split(v, ";;;")
				for _, part := range parts {
					query.Add(k, part)
				}
			}
			for k, v := range args.Headers {
				if r.Header.Values(k) != nil {
					// Ignore whitelisted header if already set.
					continue
				}
				parts := strings.Split(v, ";;;")
				for _, part := range parts {
					r.Header.Add(k, part)
				}
			}

			r = r.WithContext(WithArgs(r.Context(), args))
		}
		r.URL.RawQuery = query.Encode()

		next.ServeHTTP(sw, r)
	})
}

func extractAccessCode(r *gohttp.Request) {
	ac, host, err := auth.ExtractLeadingAccessCode(r.Host)
	if err == auth.ErrAccessCodeNotFound {
		return
	}

	r.Host = host
	r.URL.Host = host
	query := r.URL.Query()
	query.Add(http.ParamP3AccessCode, ac)
	r.URL.RawQuery = query.Encode()
}

func redirectToStarbase(w gohttp.ResponseWriter, r *gohttp.Request) {
	cfg := config.Get()
	scheme := "https"
	targetHost := cfg.ExternalServices.Starbase
	if strings.HasPrefix(targetHost, "http://") {
		scheme = "http"
		targetHost = targetHost[7:]
	} else if strings.HasPrefix(targetHost, "https://") {
		targetHost = targetHost[8:]
	}

	url := *r.URL
	url.Scheme = scheme
	url.Host = strings.Replace(
		stripPort(r.Host),
		cfg.GW3Hostname,
		targetHost,
		1,
	)
	gohttp.Redirect(w, r, url.String(), gohttp.StatusTemporaryRedirect)
}

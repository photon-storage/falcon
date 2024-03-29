package node

import (
	"context"
	"net"
	gohttp "net/http"
	"strconv"
	"strings"
	"sync"

	coreiface "github.com/ipfs/boxo/coreiface"
	options "github.com/ipfs/boxo/coreiface/options"
	"github.com/ipfs/boxo/gateway"
	cmdshttp "github.com/ipfs/go-ipfs-cmds/http"
	oldcmds "github.com/ipfs/kubo/commands"
	kuboconfig "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	corecommands "github.com/ipfs/kubo/core/commands"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/corehttp"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-gw3/common/http"

	"github.com/photon-storage/falcon/node/config"
	"github.com/photon-storage/falcon/node/handlers"
)

const (
	apiPrefix = "/api/v0"
)

type serverConfig struct {
	addr     string
	listener net.Listener
	useTLS   bool
	certFile string
	keyFile  string
}

func initFalconGateway(
	ctx context.Context,
	cctx *oldcmds.Context,
	nd *core.IpfsNode,
) (<-chan error, error) {
	cfg := config.Get()
	certFile := ""
	keyFile := ""
	if cfg.RequireTLSCert() {
		if certPath, err := findCertAndKeyFile(); err != nil {
			return nil, err
		} else {
			certFile = certPath.certFile
			keyFile = certPath.keyFile
		}
	}

	var serverCfgs []*serverConfig
	for _, la := range cfg.ListenAddresses {
		nl, err := net.Listen("tcp", la.Address)
		if err != nil {
			return nil, err
		}

		lis, err := manet.WrapNetListener(nl)
		if err != nil {
			return nil, err
		}

		serverCfgs = append(serverCfgs, &serverConfig{
			addr:     la.Address,
			listener: manet.NetListener(lis),
			useTLS:   la.UseTLS,
			certFile: certFile,
			keyFile:  keyFile,
		})
	}

	rcfg, err := nd.Repo.Config()
	if err != nil {
		return nil, err
	}
	coreapi, err := coreapi.NewCoreAPI(
		nd,
		options.Api.FetchBlocks(!rcfg.Gateway.NoFetch),
	)
	if err != nil {
		return nil, err
	}

	gwCfg := publicGatewayConfig(rcfg)
	auth, err := newAuthHandler(prepareHostnameGateways(gwCfg.PublicGateways))
	if err != nil {
		return nil, err
	}
	report := newMonitorHandler(coreapi)

	opts := []corehttp.ServeOption{
		// The order of options is important. apiOption and hostnameOption
		// share the same mux. Due to the matching rule, /status, /api/v0
		// are more specific so they get handled first. After that, it falls
		// through to the "/" handler registered by the hostnameOption, which
		// handles /ipfs or subdomain requests. The subdomain requests are
		// reformated to /ipfs and handled by the next mux registered by
		// the gatewayOption.
		apiOption(cctx, gwCfg, coreapi, auth, report),
		hostnameOption(cctx, gwCfg, auth, report),
		gatewayOption(cctx, coreapi, gwCfg, auth, report),
		corehttp.VersionOption(),
	}

	errc := make(chan error)
	var wg sync.WaitGroup
	for _, cfg := range serverCfgs {
		log.Warn("Falcon gateway starts listening", "address", cfg.addr)
		wg.Add(1)
		go func(cfg *serverConfig) {
			defer wg.Done()
			err := serveTraffic(nd, cfg, opts...)

			log.Warn("listen error", "error", err)
			errc <- err
		}(cfg)
	}

	go func() {
		wg.Wait()
		close(errc)
	}()

	return errc, nil
}

func apiOption(
	cctx *oldcmds.Context,
	gwCfg gateway.Config,
	coreapi coreiface.CoreAPI,
	auth *authHandler,
	report *monitorHandler,
) corehttp.ServeOption {
	return func(
		nd *core.IpfsNode,
		lis net.Listener,
		mux *gohttp.ServeMux,
	) (*gohttp.ServeMux, error) {
		apiHandlers := buildApiHandler(*cctx, lis)
		extHandlers := handlers.New(nd, coreapi, apiHandlers)

		mux.Handle("/status", extHandlers.Status())
		mux.Handle("/status/", extHandlers.Status())

		mux.Handle(apiPrefix+"/", auth.wrap(
			report.wrap(apiHandlers),
		))

		// Custom /api/v0 APIs.
		ch := func(next gohttp.Handler) gohttp.Handler {
			return gohttp.HandlerFunc(
				func(w gohttp.ResponseWriter, r *gohttp.Request) {
					for k, v := range gwCfg.Headers {
						w.Header()[gohttp.CanonicalHeaderKey(k)] = v
					}

					next.ServeHTTP(w, r)
				},
			)
		}

		mux.Handle(apiPrefix+"/pin/add", auth.wrap(
			report.wrap(ch(extHandlers.PinAdd())),
		))
		mux.Handle(apiPrefix+"/pin/rm", auth.wrap(
			report.wrap(ch(extHandlers.PinRm())),
		))
		mux.Handle(apiPrefix+"/pin/children_update", auth.wrap(
			report.wrap(ch(extHandlers.PinChildrenUpdate())),
		))
		mux.Handle(apiPrefix+"/pin/ls", auth.wrap(
			report.wrap(ch(extHandlers.PinList())),
		))
		mux.Handle(apiPrefix+"/pin/count", auth.wrap(
			report.wrap(ch(extHandlers.PinnedCount())),
		))
		mux.Handle(apiPrefix+"/name/broadcast", auth.wrap(
			report.wrap(ch(extHandlers.NameBroadcast())),
		))
		mux.Handle(apiPrefix+"/dag/import", auth.wrap(
			report.wrap(ch(extHandlers.DagImport())),
		))

		return mux, nil
	}
}

func hostnameOption(
	cctx *oldcmds.Context,
	gwCfg gateway.Config,
	auth *authHandler,
	report *monitorHandler,
) corehttp.ServeOption {
	return func(
		nd *core.IpfsNode,
		lis net.Listener,
		mux *gohttp.ServeMux,
	) (*gohttp.ServeMux, error) {
		backend, err := newGatewayBackend(nd)
		if err != nil {
			return nil, err
		}

		nextMux := gohttp.NewServeMux()

		hostnameHandler := gateway.NewHostnameHandler(gwCfg, backend, nextMux)
		mux.Handle("/", auth.wrap(report.wrap(
			gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
				// Skip hostname options for writable APIs.
				switch r.Method {
				case gohttp.MethodPost, gohttp.MethodDelete, gohttp.MethodPut:
					nextMux.ServeHTTP(w, r)
				default:
					hostnameHandler.ServeHTTP(w, r)
				}
			}),
		)))

		return nextMux, nil
	}
}

func gatewayOption(
	cctx *oldcmds.Context,
	coreapi coreiface.CoreAPI,
	gwCfg gateway.Config,
	auth *authHandler,
	report *monitorHandler,
) corehttp.ServeOption {
	return func(
		nd *core.IpfsNode,
		_ net.Listener,
		mux *gohttp.ServeMux,
	) (*gohttp.ServeMux, error) {
		ipfsHandler, err := buildIpfsHandler(nd, coreapi, gwCfg)
		if err != nil {
			return nil, err
		}

		mux.Handle("/ipfs/", auth.wrap(report.wrap(ipfsHandler)))
		mux.Handle("/ipns/", auth.wrap(report.wrap(ipfsHandler)))

		return mux, nil
	}
}

// The implementation is base on function GatewayOption() from
// github.com/ipfs/kubo/core/corehttp/gateway.go
func buildIpfsHandler(
	nd *core.IpfsNode,
	coreapi coreiface.CoreAPI,
	gwCfg gateway.Config,
) (gohttp.Handler, error) {
	backend, err := newGatewayBackend(nd)
	if err != nil {
		return nil, err
	}

	readable := gateway.NewHandler(gwCfg, backend)
	writable := &writableGatewayHandler{
		config: &gwCfg,
		api:    coreapi,
	}

	return gohttp.HandlerFunc(func(w gohttp.ResponseWriter, r *gohttp.Request) {
		switch r.Method {
		case gohttp.MethodPost:
			writable.handlePost(w, r)
		case gohttp.MethodDelete:
			writable.handleDelete(w, r)
		case gohttp.MethodPut:
			writable.handlePut(w, r)
		default:
			readable.ServeHTTP(w, r)
		}
	}), nil
}

// The implementation is base on function CommandsOption() from
// github.com/ipfs/kubo/core/corehttp/commands.go
func buildApiHandler(
	cctx oldcmds.Context,
	lis net.Listener,
) gohttp.Handler {
	cfg := cmdshttp.NewServerConfig()
	cfg.AllowGet = true
	cfg.APIPath = apiPrefix

	return cmdshttp.NewHandler(&cctx, corecommands.Root, cfg)
}

func patchCORSVars(cfg *cmdshttp.ServerConfig, addr net.Addr) {
	// we have to grab the port from an addr, which may be an ip6 addr.
	// TODO: this should take multiaddrs and derive port from there.
	port := ""
	if tcpaddr, ok := addr.(*net.TCPAddr); ok {
		port = strconv.Itoa(tcpaddr.Port)
	} else if udpaddr, ok := addr.(*net.UDPAddr); ok {
		port = strconv.Itoa(udpaddr.Port)
	}

	// we're listening on tcp/udp with ports. ("udp!?" you say? yeah... it happens...)
	oldOrigins := cfg.AllowedOrigins()
	newOrigins := make([]string, len(oldOrigins))
	for i, o := range oldOrigins {
		// TODO: allow replacing <host>. tricky, ip4 and ip6 and hostnames...
		if port != "" {
			o = strings.Replace(o, "<port>", port, -1)
		}
		newOrigins[i] = o
	}
	cfg.SetAllowedOrigins(newOrigins...)
}

func publicGatewayConfig(rcfg *kuboconfig.Config) gateway.Config {
	publicGws := map[string]*gateway.PublicGateway{
		"localhost": {
			Paths:                 []string{"/ipfs/", "/ipns/"},
			NoDNSLink:             rcfg.Gateway.NoDNSLink,
			UseSubdomains:         true,
			DeserializedResponses: true,
		},
		http.KnownHostNoSubdomain: {
			Paths:                 []string{"/ipfs/", "/ipns/"},
			NoDNSLink:             rcfg.Gateway.NoDNSLink,
			UseSubdomains:         false,
			DeserializedResponses: true,
		},
		config.Get().GW3Hostname: {
			Paths:                 []string{"/ipfs/", "/ipns/"},
			NoDNSLink:             rcfg.Gateway.NoDNSLink,
			UseSubdomains:         true,
			DeserializedResponses: true,
		},
	}
	// Follow the same logic from corehttp.convertPublicGateways()
	for h, gw := range rcfg.Gateway.PublicGateways {
		if gw == nil {
			delete(publicGws, h)
			continue
		}

		publicGws[h] = &gateway.PublicGateway{
			Paths:         gw.Paths,
			NoDNSLink:     gw.NoDNSLink,
			UseSubdomains: gw.UseSubdomains,
			InlineDNSLink: gw.InlineDNSLink.WithDefault(
				kuboconfig.DefaultInlineDNSLink,
			),
		}
	}

	headers := make(map[string][]string, len(rcfg.Gateway.HTTPHeaders))
	for h, v := range rcfg.Gateway.HTTPHeaders {
		headers[gohttp.CanonicalHeaderKey(h)] = v
	}
	gateway.AddAccessControlHeaders(headers)

	return gateway.Config{
		Headers:               headers,
		DeserializedResponses: true,
		PublicGateways:        publicGws,
	}
}

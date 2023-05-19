package node

import (
	"context"
	"net"
	gohttp "net/http"
	"strconv"
	"strings"
	"sync"

	cmdshttp "github.com/ipfs/go-ipfs-cmds/http"
	"github.com/ipfs/go-libipfs/gateway"
	ipfsgw "github.com/ipfs/go-libipfs/gateway"
	iface "github.com/ipfs/interface-go-ipfs-core"
	"github.com/ipfs/interface-go-ipfs-core/options"
	oldcmds "github.com/ipfs/kubo/commands"
	config "github.com/ipfs/kubo/config"
	"github.com/ipfs/kubo/core"
	corecommands "github.com/ipfs/kubo/core/commands"
	"github.com/ipfs/kubo/core/coreapi"
	"github.com/ipfs/kubo/core/corehttp"
	manet "github.com/multiformats/go-multiaddr/net"

	"github.com/photon-storage/go-common/log"
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
	cfg := Cfg()
	certFile := ""
	keyFile := ""
	if cfg.RequireTLSCert() {
		var err error
		certFile, keyFile, err = findCertAndKeyFile()
		if err != nil {
			return nil, err
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

	opts := []corehttp.ServeOption{
		gatewayOption(cctx),
		// There is a corehttp.HostnameOption() from kubo. However, that
		// option overrides mux and requires to be before gatewayOption().
		// It always redirects /ipns/{CID} request to {CID}.hostname/.
		// With hostnameOption() accepts both request types and does not
		// redirect.
		hostnameOption(cctx),
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

func hostnameOption(cctx *oldcmds.Context) corehttp.ServeOption {
	return func(
		nd *core.IpfsNode,
		_ net.Listener,
		mux *gohttp.ServeMux,
	) (*gohttp.ServeMux, error) {
		rcfg, err := nd.Repo.Config()
		if err != nil {
			return nil, err
		}

		gwAPI, err := newGatewayAPI(nd)
		if err != nil {
			return nil, err
		}

		// TODO(kmax): when NoDNSLink == false, it could crash on an invalid
		// URL. For example, http://bafybeib3ffl2teiqdncv3mkz4r23b5ctrwkzrrhctdbne6iboayxuxk5ui.localhost/root2/root3/root4/index.html
		// where .ipfs is missing before localhost. It causes stack overflow.
		// The problem can not be reproduced on desktop client with Kubo 0.20.0
		publicGws := map[string]*gateway.Specification{
			"localhost": {
				Paths:         []string{"/ipfs", "/ipns"},
				NoDNSLink:     true, // rcfg.Gateway.NoDNSLink,
				UseSubdomains: true,
			},
			Cfg().GW3Hostname: {
				Paths:         []string{"/ipfs", "/ipns"},
				NoDNSLink:     true, // rcfg.Gateway.NoDNSLink,
				UseSubdomains: true,
			},
		}

		// Follow the same logic from corehttp.convertPublicGateways()
		for h, gw := range rcfg.Gateway.PublicGateways {
			if gw == nil {
				delete(publicGws, h)
				continue
			}

			publicGws[h] = &gateway.Specification{
				Paths:         gw.Paths,
				NoDNSLink:     true, // gw.NoDNSLink,
				UseSubdomains: gw.UseSubdomains,
				InlineDNSLink: gw.InlineDNSLink.WithDefault(
					config.DefaultInlineDNSLink,
				),
			}
		}

		mux.Handle("/", gateway.WithHostname(
			mux,
			gwAPI,
			publicGws,
			rcfg.Gateway.NoDNSLink),
		)

		return mux, nil
	}
}

func gatewayOption(cctx *oldcmds.Context) corehttp.ServeOption {
	return func(
		nd *core.IpfsNode,
		lis net.Listener,
		mux *gohttp.ServeMux,
	) (*gohttp.ServeMux, error) {
		rcfg, err := nd.Repo.Config()
		if err != nil {
			return nil, err
		}

		api, err := coreapi.NewCoreAPI(
			nd,
			options.Api.FetchBlocks(!rcfg.Gateway.NoFetch),
		)
		if err != nil {
			return nil, err
		}

		auth, err := newAuthHandler()
		if err != nil {
			return nil, err
		}

		report := newReportHandler(api)

		ipfsHandler, err := buildIpfsHandler(nd, rcfg, api)
		if err != nil {
			return nil, err
		}

		extHandlers := newExtendedHandlers(nd, rcfg, api)

		mux.Handle("/ipfs/", auth.wrap(report.wrap(ipfsHandler)))
		mux.Handle("/ipns/", auth.wrap(report.wrap(ipfsHandler)))
		mux.Handle(apiPrefix+"/", auth.wrap(
			report.wrap(buildApiHandler(*cctx, lis)),
		))
		// Custom /api/v0 APIs.
		mux.Handle(apiPrefix+"/pin/count", auth.wrap(
			report.wrap(extHandlers.pinnedCount()),
		))

		mux.Handle("/status", extHandlers.status())
		mux.Handle("/status/", extHandlers.status())

		return mux, nil
	}
}

// The implementation is base on function GatewayOption() from
// github.com/ipfs/kubo/core/corehttp/gateway.go
func buildIpfsHandler(
	nd *core.IpfsNode,
	rcfg *config.Config,
	api iface.CoreAPI,
) (gohttp.Handler, error) {
	headers := make(map[string][]string, len(rcfg.Gateway.HTTPHeaders))
	for h, v := range rcfg.Gateway.HTTPHeaders {
		headers[gohttp.CanonicalHeaderKey(h)] = v
	}
	ipfsgw.AddAccessControlHeaders(headers)
	gwCfg := ipfsgw.Config{
		Headers: headers,
	}

	gwAPI, err := newGatewayAPI(nd)
	if err != nil {
		return nil, err
	}

	readable := ipfsgw.NewHandler(gwCfg, gwAPI)
	writable := &writableGatewayHandler{
		config: &gwCfg,
		api:    api,
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
	cfg.SetAllowedMethods(
		gohttp.MethodGet,
		gohttp.MethodPost,
		gohttp.MethodPut,
	)
	cfg.APIPath = apiPrefix

	// NOTE(kmax): seems not relevant.
	// addHeadersFromConfig(cfg, rcfg)
	// addCORSFromEnv(cfg)
	addCORSDefaults(cfg)
	patchCORSVars(cfg, lis.Addr())

	return cmdshttp.NewHandler(&cctx, corecommands.Root, cfg)
}

func addCORSDefaults(cfg *cmdshttp.ServerConfig) {
	// always safelist certain origins
	cfg.AppendAllowedOrigins(
		"http://127.0.0.1:<port>",
		"https://127.0.0.1:<port>",
		"http://[::1]:<port>",
		"https://[::1]:<port>",
		"http://localhost:<port>",
		"https://localhost:<port>",
	)
	cfg.AppendAllowedOrigins(
		"chrome-extension://nibjojkomfdiaoajekhjakgkdhaomnch", // ipfs-companion
		"chrome-extension://hjoieblefckbooibpepigmacodalfndh", // ipfs-companion-beta
	)
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

package node

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	logging "github.com/ipfs/go-log"
	core "github.com/ipfs/kubo/core"
	"github.com/ipfs/kubo/core/corehttp"
	"github.com/jbenet/goprocess"
	periodicproc "github.com/jbenet/goprocess/periodic"
	manet "github.com/multiformats/go-multiaddr/net"
)

// ***********************************************************************//
// copied from github.com/ipfs/kubo/core/corehttp/corehttp.go
// with code format and change to support TLS.
// ***********************************************************************//
var ipfslogger = logging.Logger("core/server")

// shutdownTimeout is the timeout after which we'll stop waiting for hung
// commands to return on shutdown.
const shutdownTimeout = 30 * time.Second

// makeHandler turns a list of ServeOptions into a http.Handler that implements
// all of the given options, in order.
func makeHandler(
	n *core.IpfsNode,
	l net.Listener,
	options ...corehttp.ServeOption,
) (http.Handler, error) {
	topMux := http.NewServeMux()
	mux := topMux
	for _, option := range options {
		var err error
		mux, err = option(n, l, mux)
		if err != nil {
			return nil, err
		}
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ServeMux does not support requests with CONNECT method,
		// so we need to handle them separately
		// https://golang.org/src/net/http/request.go#L111
		if r.Method == http.MethodConnect {
			w.WriteHeader(http.StatusOK)
			return
		}
		topMux.ServeHTTP(w, r)
	})
	return handler, nil
}

// serveTraffic accepts incoming HTTP connections on the listener and pass them
// to ServeOption handlers.
func serveTraffic(
	nd *core.IpfsNode,
	cfg *serverConfig,
	options ...corehttp.ServeOption,
) error {
	// make sure we close this no matter what.
	defer cfg.listener.Close()

	handler, err := makeHandler(nd, cfg.listener, options...)
	if err != nil {
		return err
	}

	addr, err := manet.FromNetAddr(cfg.listener.Addr())
	if err != nil {
		return err
	}

	select {
	case <-nd.Process.Closing():
		return fmt.Errorf("failed to start server, process closing")
	default:
	}

	server := &http.Server{
		Handler: handler,
	}

	var serverError error
	serverProc := nd.Process.Go(func(p goprocess.Process) {
		if cfg.useTLS {
			serverError = server.ServeTLS(
				cfg.listener,
				cfg.certFile,
				cfg.keyFile,
			)
		} else {
			serverError = server.Serve(cfg.listener)
		}
	})

	// wait for server to exit.
	select {
	case <-serverProc.Closed():
	// if node being closed before server exits, close server
	case <-nd.Process.Closing():
		ipfslogger.Infof("server at %s terminating...", addr)

		warnProc := periodicproc.Tick(5*time.Second, func(_ goprocess.Process) {
			ipfslogger.Infof("waiting for server at %s to terminate...", addr)
		})

		// This timeout shouldn't be necessary if all of our commands
		// are obeying their contexts but we should have *some* timeout.
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		err := server.Shutdown(ctx)

		// Should have already closed but we still need to wait for it
		// to set the error.
		<-serverProc.Closed()
		serverError = err

		warnProc.Close()
	}

	ipfslogger.Infof("server at %s terminated", addr)
	return serverError
}

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"

	"github.com/photon-storage/go-gw3/common/crypto"
)

type httpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type config struct {
	host string
	port int
	cli  httpClient
}

type check struct {
	skip bool
	stop bool
	desc string
	run  func(ctx context.Context, cfg config) error
}

func main() {
	cfg := config{
		cli: newAuthClient(crypto.PregenEd25519(0)),
	}

	flag.StringVar(&cfg.host, "host", "localhost", "host to request")
	flag.IntVar(&cfg.port, "port", 8080, "port to request")
	flag.Parse()

	ctx := context.Background()

	for _, c := range checks {
		if c.skip {
			continue
		}

		fmt.Printf("--------------------------------------------------\n")
		fmt.Printf("Check: %v\n", c.desc)
		if err := c.run(ctx, cfg); err != nil {
			fmt.Printf("!!! Check FAILED: %v\n", err)
			return
		}

		if c.stop {
			break
		}
	}
	fmt.Printf("!!! All checks PASSED!\n")
}

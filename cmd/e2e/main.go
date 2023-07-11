package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-gw3/common/crypto"
)

type config struct {
	host string
	port int
	cli  *authClient
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
	flag.IntVar(&cfg.port, "port", 80, "port to request")
	flag.Parse()

	ctx := context.Background()

	for _, c := range checks {
		if c.skip {
			continue
		}

		fmt.Printf("--------------------------------------------------\n")
		fmt.Printf("%v\n", log.Blue(fmt.Sprintf("Run: %v", c.desc)))
		if err := c.run(ctx, cfg); err != nil {
			fmt.Printf("%v\n", log.Red(fmt.Sprintf("FAILED: %v", err)))
			return
		} else {
			fmt.Printf("%v\n", log.Green(fmt.Sprintf("PASSED")))
		}

		if c.stop {
			break
		}
	}
	fmt.Printf("%v\n", log.Green(fmt.Sprintf("All checks PASSED")))
}

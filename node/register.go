package node

import (
	"context"
	"fmt"
	gohttp "net/http"
	"net/url"
	"time"

	"github.com/ipfs/kubo/core"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/http"
)

func registerFalconNode(ctx context.Context, nd *core.IpfsNode) {
	ticker := time.NewTicker(60 * time.Second)
	cfg := Cfg()
	for {
		select {
		case <-ticker.C:
			req, err := gohttp.NewRequest(
				gohttp.MethodPost,
				fmt.Sprintf(
					"%v/gateway/register?pk=%v&host=%v&port=%v",
					cfg.ExternalServices.Starbase,
					url.QueryEscape(cfg.PublicKeyBase64),
					cfg.Discovery.PublicHost,
					cfg.Discovery.PublicPort,
				),
				nil,
			)
			if err != nil {
				log.Error("Error creating registration request", "error", err)
				// Return as this is not fixable with retry.
				return
			}

			if err := auth.SignRequest(
				req,
				http.NewArgs().
					SetArg(http.ArgP3Node, cfg.PublicKeyBase64).
					SetArg(
						http.ArgP3Unixtime,
						fmt.Sprintf("%v", time.Now().Unix()),
					),
				cfg.SecretKey,
			); err != nil {
				log.Error("Error signing registration request", "error", err)
				// Return as this is not fixable with retry.
				return
			}

			resp, err := cfg.HttpClient.Do(req)
			if err != nil {
				log.Error("Error registering falcon node", "error", err)
				break
			}
			if resp.StatusCode != gohttp.StatusOK {
				log.Error("Unexpected response code",
					"code", resp.StatusCode,
				)
				break
			}

			log.Info("Falcon node registration successful",
				"host", cfg.Discovery.PublicHost,
				"port", cfg.Discovery.PublicPort,
			)
			return

		case <-ctx.Done():
			return
		}
	}
}

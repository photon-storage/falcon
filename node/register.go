package node

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	gohttp "net/http"
	"net/url"
	"time"

	"github.com/ipfs/kubo/core"

	"github.com/photon-storage/go-common/log"
	"github.com/photon-storage/go-gw3/common/auth"
	"github.com/photon-storage/go-gw3/common/http"
)

type Cert struct {
	PrivateKey  string    `json:"priv_key"`
	Certificate string    `json:"cert"`
	At          time.Time `json:"time"`
}

type RegisterResult struct {
	Status string `json:"status"`
	Cert   *Cert  `json:"certificate,omitempty"`
}

func registerFalconNode(ctx context.Context, nd *core.IpfsNode) error {
	cfg := Cfg()

	// Purge expired certficates and retrieve from starbase if none exists.
	fetchCert := false
	if cfg.RequireTLSCert() {
		if err := purgeExpiredCerts(); err != nil {
			return err
		}
		if _, err := findCertAndKeyFile(); err != nil {
			if err == ErrCertNotFound {
				fetchCert = true
			} else {
				return err
			}
		}
	}

	ticker := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-ticker.C:
			req, err := gohttp.NewRequest(
				gohttp.MethodPost,
				fmt.Sprintf(
					"%v/gateway/register",
					cfg.ExternalServices.Starbase,
				),
				nil,
			)
			if err != nil {
				log.Error("Error creating registration request", "error", err)
				// Return as this is not fixable with retry.
				return err
			}

			query := url.Values{}
			query.Set("pk", cfg.PublicKeyBase64)
			query.Set("host", cfg.Discovery.PublicHost)
			query.Set("port", fmt.Sprintf("%v", cfg.Discovery.PublicPort))
			if fetchCert {
				query.Set("cert", "true")
			}
			req.URL.RawQuery = query.Encode()

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
				return err
			}

			resp, err := cfg.HttpClient.Do(req)
			if err != nil {
				log.Error("Error registering falcon node", "error", err)
				break
			}
			defer resp.Body.Close()

			if resp.StatusCode != gohttp.StatusOK {
				log.Error("Unexpected response code",
					"code", resp.StatusCode,
				)
				break
			}

			enc, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				log.Error("Error reading response body", "error", err)
				break
			}
			var res RegisterResult
			if err := json.Unmarshal(enc, &res); err != nil {
				log.Error("Error decoding starbase response", "error", err)
				break
			}
			if res.Status != "ok" {
				log.Error("Starbase responds with non-ok status",
					"status", res.Status,
				)
				break
			}

			if res.Cert != nil {
				if err := saveCert(
					[]byte(res.Cert.PrivateKey),
					[]byte(res.Cert.Certificate),
					res.Cert.At,
				); err != nil {
					log.Error("Error saving certificate", "error", err)
					break
				}
			}

			done := true
			if fetchCert {
				if _, err := findCertAndKeyFile(); err != nil {
					done = false
				}
			}

			if done {
				log.Info("Falcon node registration successful",
					"host", cfg.Discovery.PublicHost,
					"port", cfg.Discovery.PublicPort,
				)
				return nil
			} else {
				log.Info("Waiting for TLS certificate to become available")
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

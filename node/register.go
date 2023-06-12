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
	Status            string `json:"status"`
	Cert              *Cert  `json:"certificate,omitempty"`
	RequireCertUpdate bool   `json:"require_cert_update"`
}

func registerFalconNode(ctx context.Context, _ *core.IpfsNode) error {
	cfg := Cfg()

	// Purge expired certficates and retrieve from starbase if none exists.
	fetchCert := false
	if cfg.RequireTLSCert() {
		if err := purgeExpiredCerts(true); err != nil {
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

	register := func() (bool, error) {
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
			return true, err
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
			return true, err
		}

		resp, err := cfg.HttpClient.Do(req)
		if err != nil {
			log.Warn("Error registering falcon node", "error", err)
			return false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != gohttp.StatusOK {
			log.Warn("Unexpected response code",
				"code", resp.StatusCode,
			)
			return false, nil
		}

		enc, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Warn("Error reading response body", "error", err)
			return false, nil
		}
		var res RegisterResult
		if err := json.Unmarshal(enc, &res); err != nil {
			log.Warn("Error decoding starbase response", "error", err)
			return false, nil
		}
		if res.Status != "ok" {
			log.Warn("Starbase responds with non-ok status",
				"status", res.Status,
			)
			return false, nil
		}

		// Did not request cert but starbase requires a refresh.
		// This is triggered by host IP change. Delete all certs as
		// domain has changed. If fetchCert was true, all certs should
		// have been purged already (resulting in a not found err)
		if !fetchCert && res.RequireCertUpdate {
			if err := purgeExpiredCerts(false); err != nil {
				return true, err
			}
			fetchCert = true
		}

		if res.Cert != nil {
			if err := saveCert(
				[]byte(res.Cert.PrivateKey),
				[]byte(res.Cert.Certificate),
				res.Cert.At,
			); err != nil {
				log.Warn("Error saving certificate", "error", err)
				return false, nil
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
			return true, nil
		} else {
			log.Info("Waiting for TLS certificate to become available")
			return false, nil
		}
	}

	done, err := register()
	if err != nil {
		return err
	}
	if done {
		return nil
	}

	ticker := time.NewTicker(60 * time.Second)
	for {
		select {
		case <-ticker.C:
			done, err := register()
			if err != nil {
				return err
			}
			if done {
				return nil
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

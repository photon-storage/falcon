package node

import (
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/photon-storage/go-common/log"

	"github.com/photon-storage/falcon/node/cert"
)

const (
	keyFileName    = "falcon.key"
	certFileName   = "falcon.crt"
	certExpiration = 20 * 24 * time.Hour
)

func certsPath() string {
	return filepath.Join(falconPath(), "certs")
}

func subdir(ts int64) string {
	return filepath.Join(certsPath(), fmt.Sprintf("%v", ts))
}

func findLatest() (int64, error) {
	fis, err := ioutil.ReadDir(certsPath())
	if err != nil {
		return 0, err
	}

	var names []int64
	for _, fi := range fis {
		if !fi.IsDir() {
			continue
		}
		ts, err := strconv.ParseInt(fi.Name(), 10, 64)
		if err != nil {
			continue
		}

		names = append(names, ts)
	}

	if len(names) == 0 {
		return 0, nil
	}

	sort.Slice(names, func(i, j int) bool {
		return names[i] > names[j]
	})

	return names[0], nil
}

func purgeSubdirs(keep string) error {
	fis, err := ioutil.ReadDir(certsPath())
	if err != nil {
		return err
	}

	for _, fi := range fis {
		if fi.Name() == keep {
			continue
		}

		os.RemoveAll(filepath.Join(certsPath(), fi.Name()))
	}

	return nil
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func refreshTLSCert() error {
	cfg := Cfg()

	h := sha256.Sum256([]byte(cfg.PublicKeyBase64))
	email := fmt.Sprintf("%x@gw3.io", h[:8])

	if err := os.MkdirAll(certsPath(), 0644); err != nil {
		return err
	}

	ts, err := findLatest()
	if err != nil {
		return err
	}

	now := time.Now()
	if ts > 0 && now.Sub(time.Unix(ts, 0)) < certExpiration {
		dir := subdir(ts)
		if fileExists(filepath.Join(dir, keyFileName)) &&
			fileExists(filepath.Join(dir, certFileName)) {
			return nil
		}
	}

	log.Warn("TLS cert and key expired or non-exist, creating")

	pemSk, pemCert, err := cert.ObtainCert(email, cfg.GW3Hostname, nil)
	if err != nil {
		return err
	}

	dir := subdir(now.Unix())
	if err := os.MkdirAll(dir, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(dir, keyFileName),
		pemSk,
		0644); err != nil {
		return err
	}
	if err := os.WriteFile(
		filepath.Join(dir, certFileName),
		pemCert,
		0644); err != nil {
		return err
	}

	purgeSubdirs(fmt.Sprintf("%v", now.Unix()))

	return nil
}

func findCertAndKeyFile() (string, string, error) {
	ts, err := findLatest()
	if err != nil {
		return "", "", err
	}

	dir := subdir(ts)
	certFile := filepath.Join(dir, certFileName)
	keyFile := filepath.Join(dir, keyFileName)

	if !fileExists(certFile) {
		return "", "", fmt.Errorf("cert file is missing")
	}
	if !fileExists(keyFile) {
		return "", "", fmt.Errorf("key file is missing")
	}

	return certFile, keyFile, nil
}

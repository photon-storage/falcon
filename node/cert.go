package node

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"
)

const (
	keyFileName    = "falcon.key"
	certFileName   = "falcon.crt"
	certExpiration = 20 * 24 * time.Hour
)

var (
	ErrCertNotFound = errors.New("certificate not found")
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

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func saveCert(pemSk, pemCert []byte, at time.Time) error {
	if err := os.MkdirAll(certsPath(), 0644); err != nil {
		return err
	}

	dir := subdir(at.Unix())
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

	return nil
}

func purgeExpiredCerts(keepLatest bool) error {
	ts, err := findLatest()
	if err != nil {
		return err
	}

	if ts == 0 {
		return nil
	}

	keep := ""
	if keepLatest && time.Since(time.Unix(ts, 0)) < certExpiration {
		keep = fmt.Sprintf("%v", ts)
	}

	fis, err := ioutil.ReadDir(certsPath())
	if err != nil {
		return err
	}

	for _, fi := range fis {
		if fi.Name() == keep {
			continue
		}

		if err := os.RemoveAll(
			filepath.Join(certsPath(), fi.Name()),
		); err != nil {
			return err
		}
	}

	return nil
}

type certPath struct {
	certFile string
	keyFile  string
}

func findCertAndKeyFile() (*certPath, error) {
	ts, err := findLatest()
	if err != nil {
		return nil, err
	}

	if ts == 0 {
		return nil, ErrCertNotFound
	}

	dir := subdir(ts)
	certFile := filepath.Join(dir, certFileName)
	keyFile := filepath.Join(dir, keyFileName)

	if !fileExists(certFile) {
		return nil, ErrCertNotFound
	}
	if !fileExists(keyFile) {
		return nil, ErrCertNotFound
	}

	return &certPath{
		certFile: certFile,
		keyFile:  keyFile,
	}, nil
}

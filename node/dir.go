package node

import (
	"os"
	"path/filepath"
)

func ensureFalconPath() error {
	if err := os.MkdirAll(falconPath(), 0644); err != nil {
		return err
	}
	return os.MkdirAll(certsPath(), 0644)
}

func falconPath() string {
	p := os.Getenv("FALCON_PATH")
	if p == "" {
		p = filepath.Join(os.Getenv("HOME"), ".falcon")
	}
	return p
}

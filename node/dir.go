package node

import (
	"os"
	"path/filepath"
)

func ensureFalconPath() error {
	return os.MkdirAll(falconPath(), 0644)
}

func falconPath() string {
	p := os.Getenv("FALCON_PATH")
	if p == "" {
		p = filepath.Join(os.Getenv("HOME"), ".falcon")
	}
	return p
}

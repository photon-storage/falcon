.PHONY: falcon checks

default: falcon

falcon:
	go run ./cmd/node/. daemon --init --falcon-config=./cmd/node/config/config_dev.yaml

checks:
	go run ./cmd/node/e2e/.

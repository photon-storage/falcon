.PHONY: falcon checks

default: loner

prod:
	go run ./cmd/node/. daemon --init --falcon-config=./cmd/node/config/config_prod.yaml

dev:
	go run ./cmd/node/. daemon --init --falcon-config=./cmd/node/config/config_dev.yaml

loner:
	go run ./cmd/node/. daemon --init --falcon-config=./cmd/node/config/config_loner.yaml

checks:
	go run ./cmd/e2e/.

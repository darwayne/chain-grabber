SHELL=/bin/bash -o pipefail
pwd=$(shell pwd)

project_module=github.com/darwayne/chain-grabber

grabby-binary-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o build/grabby-linux "$(project_module)/cmd/grabby"

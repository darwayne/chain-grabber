SHELL=/bin/bash -o pipefail
pwd=$(shell pwd)

project_module=github.com/darwayne/chain-grabber

grabby-binary-linux:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -o build/grabby-linux "$(project_module)/cmd/grabby"

# zig can be installed via brew
build-zig:
	CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC="zig cc -target x86_64-linux-musl" CXX="zig c++ -target x86_64-linux-musl" go build -trimpath -o build/grabby-linux "$(project_module)/cmd/grabby"

deploy:
	scp build/grabby-linux home-server:./grabby

build-deploy: build-zig deploy

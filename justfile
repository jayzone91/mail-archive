set windows-shell := ["powershell.exe", "-NoProfile", "-Command"]

build: prepare
    bun scripts/build.ts build

build-windows: prepare
    bun scripts/build.ts build windows

build-linux: prepare
    bun scripts/build.ts build linux

build-all: prepare
    bun scripts/build.ts build windows linux

run: build
    bun scripts/build.ts run

run-windows: build-windows
    bun scripts/build.ts run windows

run-linux: build-linux
    bun scripts/build.ts run linux

dev: prepare
    go run ./cmd/mailarchive

test: prepare
    go test ./...

lint: prepare
    go vet ./...

smoke: build
    bun scripts/smoke.ts

typecheck: dependencies
    bun run typecheck

format: prepare
    go fmt ./...
    go tool templ fmt .

clean:
    bun scripts/build.ts clean

prepare: dependencies
    bun scripts/build.ts assets
    go tool templ generate

dependencies:
    bun install --frozen-lockfile

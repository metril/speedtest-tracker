BINARY   := speedtest-tracker
VERSION  ?= dev
LDFLAGS  := -s -w -X main.version=$(VERSION)

.PHONY: all build web-build go-build test go-test web-test dev lint clean

all: build

## build: build the web UI into internal/web/dist, then the Go binary
build: web-build go-build

web-build:
	cd web && npm ci && npm run build
	git checkout -- internal/web/dist/.gitkeep

go-build:
	CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)' -o $(BINARY) ./cmd/speedtest-tracker

## test: run Go and web test suites
test: go-test web-test

go-test:
	go vet ./...
	go test ./... -race

web-test:
	cd web && npm test

## dev: run the Go server and the Vite dev server side by side
dev:
	go run ./cmd/speedtest-tracker & cd web && npm run dev

## lint: go vet plus TypeScript type checking
lint:
	go vet ./...
	cd web && npm run lint

clean:
	rm -f $(BINARY)
	rm -rf internal/web/dist/assets internal/web/dist/index.html

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/hakobune8/arun/internal/cli.Version=$(VERSION)"
BINARY := arun

.PHONY: build test lint clean cover install run vet all web-build web-lint web-smoke live-validate live-orchestrate

all: lint build test

build:
	$(MAKE) web-build
	go build $(LDFLAGS) -o $(BINARY) ./cmd/arun

test:
	go test ./... -v -count=1

vet:
	go vet ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...
	$(MAKE) web-lint

web-build:
	cd web && npm ci && npm run build

web-lint:
	cd web && npm ci && npm run lint

web-smoke:
	cd web && npm ci && npm run build && npm run smoke

live-validate:
	scripts/live-validate-buildkit.sh

live-orchestrate:
	scripts/live-validate-orchestrate.sh

clean:
	rm -f $(BINARY) $(BINARY).exe coverage.out coverage.html cover.html
	rm -rf dist/ build/ tmp/
	rm -rf web/dist web/node_modules

install:
	go install $(LDFLAGS) ./cmd/arun

cover:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -html=coverage.out -o coverage.html

run:
	go run $(LDFLAGS) ./cmd/arun

docker-build:
	docker build -t arun:latest -f Dockerfile .

docker-run:
	docker run --rm -it \
		-v $$(pwd):/workspace \
		-p 8080:8080 \
		--env-file .env \
		arun:latest

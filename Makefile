VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -ldflags "-X github.com/kazyamaz200/agentos/internal/cli.Version=$(VERSION)"
BINARY := agentos

.PHONY: build test lint clean cover install run vet all

all: lint build test

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/agentos

test:
	go test ./... -v -count=1

vet:
	go vet ./...

lint:
	test -z "$$(gofmt -l .)"
	go vet ./...

clean:
	rm -f $(BINARY) $(BINARY).exe coverage.out coverage.html cover.html
	rm -rf dist/ build/ tmp/

install:
	go install $(LDFLAGS) ./cmd/agentos

cover:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -html=coverage.out -o coverage.html

run:
	go run $(LDFLAGS) ./cmd/agentos

docker-build:
	docker build -t agentos:latest -f Dockerfile .

docker-run:
	docker run --rm -it \
		-v $$(pwd):/workspace \
		-p 8080:8080 \
		--env-file .env \
		agentos:latest

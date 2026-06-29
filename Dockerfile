FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -ldflags "-X github.com/kazyamaz200/agentos/internal/cli.Version=${VERSION}" -o agentos ./cmd/agentos

FROM golang:1.22-alpine

RUN apk add --no-cache ca-certificates tzdata bash git
RUN addgroup -S agentos && adduser -S agentos -G agentos
RUN mkdir -p /workspace /home/agentos/.agentos && chown -R agentos:agentos /workspace /home/agentos

WORKDIR /workspace
COPY --from=builder /build/agentos /usr/local/bin/agentos
USER agentos
ENV HOME=/home/agentos
ENV AGENTOS_HOME=/home/agentos/.agentos

EXPOSE 8080

ENTRYPOINT ["agentos"]
CMD ["--help"]

FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates nodejs npm

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN cd web && npm ci && npm run build
ARG TARGETOS=linux
ARG TARGETARCH=amd64
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-X github.com/kazyamaz200/agentos/internal/cli.Version=${VERSION}" -o agentos ./cmd/agentos

FROM golang:1.22-alpine

ARG TARGETARCH=amd64
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apk add --no-cache ca-certificates tzdata bash git curl docker-cli && \
    case "${TARGETARCH}" in \
      amd64|arm64) helm_arch="${TARGETARCH}" ;; \
      arm) helm_arch="arm" ;; \
      *) echo "unsupported helm arch: ${TARGETARCH}" >&2; exit 1 ;; \
    esac && \
    curl -fsSL "https://get.helm.sh/helm-v3.15.4-linux-${helm_arch}.tar.gz" -o /tmp/helm.tar.gz && \
    tar -xzf /tmp/helm.tar.gz -C /tmp && \
    mv "/tmp/linux-${helm_arch}/helm" /usr/local/bin/helm && \
    chmod +x /usr/local/bin/helm && \
    ln -sf /usr/local/go/bin/go /usr/local/bin/go && \
    ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt && \
    rm -rf /tmp/helm.tar.gz "/tmp/linux-${helm_arch}"
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

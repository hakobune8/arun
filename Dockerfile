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
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -ldflags "-X github.com/hakobune8/arun/internal/cli.Version=${VERSION}" -o arun ./cmd/arun

FROM golang:1.22-alpine

ARG TARGETARCH=amd64
ENV PATH="/usr/local/go/bin:${PATH}"

RUN apk add --no-cache ca-certificates tzdata bash git curl docker-cli nodejs npm && \
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
RUN addgroup -S arun && adduser -S arun -G arun
RUN mkdir -p /workspace /home/arun/.arun && chown -R arun:arun /workspace /home/arun

WORKDIR /workspace
COPY --from=builder /build/arun /usr/local/bin/arun
USER arun
ENV HOME=/home/arun
ENV ARUN_HOME=/home/arun/.arun

EXPOSE 8080

ENTRYPOINT ["arun"]
CMD ["--help"]

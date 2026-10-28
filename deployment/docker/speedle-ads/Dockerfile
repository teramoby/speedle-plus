ARG GO_VERSION=1.26.10
FROM golang:${GO_VERSION}-alpine3.23 AS builder
ARG VERSION=dev
ARG GIT_COMMIT=unknown
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY api ./api
COPY cmd ./cmd
COPY pkg ./pkg
COPY 3rdparty ./3rdparty
RUN --mount=type=cache,target=/root/.cache/go-build GOMAXPROCS=2 CGO_ENABLED=0 go build -p=2 -trimpath -ldflags "-s -w -X main.gitCommit=${GIT_COMMIT} -X main.productVersion=${VERSION} -X main.goVersion=$(go env GOVERSION)" -o /speedle-ads ./cmd/speedle-ads

FROM alpine:3.23
RUN apk add --no-cache ca-certificates && \
    addgroup -g 1000 speedle && \
    adduser -u 1000 -G speedle -s /bin/sh -D speedle && \
    mkdir -p /var/lib/speedle && \
    printf '{}\n' > /var/lib/speedle/policies.json && \
    chown -R speedle:speedle /var/lib/speedle
COPY --from=builder /speedle-ads /usr/local/bin/speedle-ads
COPY --chmod=755 deployment/docker/speedle-ads/entrypoint.sh /usr/local/bin/entrypoint.sh
ENV SPDL_ENDPOINT=0.0.0.0:6734 \
    SPDL_FILESTORE_LOC=/var/lib/speedle/policies.json \
    SPDL_HEALTH_ENDPOINT=127.0.0.1:8082
EXPOSE 6734 50002
USER speedle
HEALTHCHECK --interval=10s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -q -T 2 -O /dev/null http://127.0.0.1:8082/readyz || exit 1
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
LABEL org.opencontainers.image.source="https://github.com/teramoby/speedle-plus" \
      org.opencontainers.image.title="Speedle ADS" \
      org.opencontainers.image.licenses="UPL-1.0"

ARG GO_VERSION=1.22
FROM golang:${GO_VERSION}-alpine AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags "-s -w -X main.gitCommit=unknown -X main.productVersion=0.1.0 -X main.goVersion=${GO_VERSION}" -o /speedle-pms ./cmd/speedle-pms

FROM alpine:3.20

RUN addgroup -g 1000 speedle && \
    adduser -u 1000 -G speedle -s /bin/sh -D speedle

RUN mkdir -p /var/lib/speedle && \
    echo "{}" > /var/lib/speedle/policies.json && \
    chown -R speedle:speedle /var/lib/speedle

COPY --from=builder /speedle-pms /usr/local/bin/speedle-pms
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 6733
ENV PATH="/usr/local/bin:${PATH}"

USER speedle

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:6733/health || exit 1

ENTRYPOINT ["entrypoint.sh"]

# OCI labels
LABEL org.opencontainers.image.source="https://github.com/teramoby/speedle-plus" \
      org.opencontainers.image.title="Speedle PMS" \
      org.opencontainers.image.description="Speedle Policy Management Service" \
      org.opencontainers.image.licenses="UPL-1.0"

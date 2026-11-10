# Built by the Release workflow. The build context contains the cross-compiled
# speedle-pms, speedle-ads, spctl and dispatch binaries for each target
# architecture, plus data/.keep, so the unprivileged user owns a writable
# directory for the file store.
FROM scratch
# Link the GHCR package to this repository so its permissions are manageable.
LABEL org.opencontainers.image.source="https://github.com/teramoby/speedle-plus"
ARG TARGETARCH
COPY --chown=65532:65532 data/ /data/
COPY speedle-pms-linux-${TARGETARCH} /speedle-pms
COPY speedle-ads-linux-${TARGETARCH} /speedle-ads
COPY spctl-linux-${TARGETARCH} /spctl
COPY dispatch-linux-${TARGETARCH} /dispatch
EXPOSE 6733 6734
USER 65532:65532
# No shell in scratch. /dispatch prints the command names, or execs the named one.
ENTRYPOINT ["/dispatch"]

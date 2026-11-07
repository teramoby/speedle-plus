# Built by the Release workflow. The build context contains the cross-compiled
# binary for each target architecture and data/.keep, so the unprivileged user
# owns a writable directory for the file store.
FROM scratch
ARG TARGETARCH
COPY --chown=65532:65532 data/ /data/
COPY speedle-ads-linux-${TARGETARCH} /speedle-ads
EXPOSE 6734
USER 65532:65532
ENTRYPOINT ["/speedle-ads"]

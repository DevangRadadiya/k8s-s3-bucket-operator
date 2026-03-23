# Final stage — distroless for minimal attack surface
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY bin/manager /manager

# Run as non-root (required for OpenShift)
USER 65532:65532

ENTRYPOINT ["/manager"]

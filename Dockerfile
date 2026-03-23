# Build stage
# Using golang:1.25-alpine once it is available, or using GOTOOLCHAIN=auto
FROM golang:1.22-alpine AS builder

ENV GOTOOLCHAIN=auto

WORKDIR /workspace

# Cache dependencies before copying source
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build
COPY cmd/ cmd/
COPY internal/ internal/
COPY api/ api/

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" -o /manager ./cmd/main.go

# Final stage — distroless for minimal attack surface
FROM gcr.io/distroless/static:nonroot

WORKDIR /

COPY --from=builder /manager /manager

# Run as non-root (required for OpenShift)
USER 65532:65532

ENTRYPOINT ["/manager"]

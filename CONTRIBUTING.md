# Contributing to k8s-s3-bucket-operator

Thanks for your interest in contributing. This guide covers everything you need to get started.

---

## Table of Contents

- [Prerequisites](#prerequisites)
- [Project Structure](#project-structure)
- [Local Development Setup](#local-development-setup)
- [Running the Operator Locally](#running-the-operator-locally)
- [Running Tests](#running-tests)
- [Submitting a Pull Request](#submitting-a-pull-request)
- [Code Style](#code-style)
- [Areas Most Needed](#areas-most-needed)

---

## Prerequisites

| Tool | Version | Purpose |
|---|---|---|
| Go | 1.21+ | Build the operator |
| Docker | 20.10+ | Build container image |
| kubectl | 1.25+ | Interact with cluster |
| kind or minikube | latest | Local Kubernetes cluster |
| MinIO | latest | Local object storage backend |

---

## Project Structure

```
k8s-s3-bucket-operator/
├── cmd/
│   └── main.go                  # Entry point — starts gRPC server
├── internal/
│   ├── driver/
│   │   └── driver.go            # COSI gRPC driver implementation
│   └── minio/
│       └── client.go            # MinIO admin client (buckets, users, policies)
├── deploy/
│   ├── cosi-controller.yaml     # Official COSI controller deployment
│   ├── operator.yaml            # Our driver deployment
│   └── openshift/
│       ├── scc.yaml             # OpenShift Security Context Constraint
│       └── operator.yaml        # OpenShift-specific driver deployment
├── config/
│   ├── rbac/
│   │   ├── role.yaml
│   │   └── clusterrole.yaml
│   └── samples/
│       ├── bucketclass.yaml     # Example BucketClass
│       └── bucketclaim.yaml     # Example BucketClaim
├── Dockerfile
├── Makefile
├── go.mod
├── README.md
└── CONTRIBUTING.md
```

---

## Local Development Setup

### 1. Clone the repo

```bash
git clone https://github.com/DevangRadadiya/k8s-s3-bucket-operator.git
cd k8s-s3-bucket-operator
```

### 2. Install Go dependencies

```bash
go mod tidy
```

### 3. Start a local Kubernetes cluster

```bash
# Using kind
kind create cluster --name cosi-dev

# OR using minikube
minikube start
```

### 4. Start MinIO locally

```bash
docker run -d \
  -p 9000:9000 \
  -p 9001:9001 \
  --name minio-dev \
  -e MINIO_ROOT_USER=minioadmin \
  -e MINIO_ROOT_PASSWORD=minioadmin \
  minio/minio server /data --console-address ":9001"
```

### 5. Install the COSI controller

```bash
kubectl apply -f deploy/cosi-controller.yaml
```

---

## Running the Operator Locally

### Set required environment variables

```bash
export MINIO_ENDPOINT=localhost:9000
export MINIO_ACCESS_KEY=minioadmin
export MINIO_SECRET_KEY=minioadmin
export MINIO_USE_SSL=false
```

### Run the driver

```bash
go run ./cmd/main.go
```

### Apply sample resources

```bash
kubectl apply -f config/samples/bucketclass.yaml
kubectl apply -f config/samples/bucketclaim.yaml
```

### Check status

```bash
kubectl get bucketclaim
kubectl get bucket
kubectl describe bucketclaim my-app-images
```

---

## Running Tests

```bash
# Unit tests
go test ./...

# With verbose output
go test -v ./...

# With coverage
go test -cover ./...
```

---

## Building the Docker Image

```bash
make docker-build IMG=ghcr.io/DevangRadadiya/k8s-s3-bucket-operator:dev
```

---

## Submitting a Pull Request

1. Fork the repo and create a branch from `main`

   ```bash
   git checkout -b feat/my-feature
   ```

2. Make your changes

3. Run tests

   ```bash
   go test ./...
   go vet ./...
   ```

4. Commit with a clear message

   ```bash
   git commit -m "feat: add Ceph RGW backend support"
   ```

5. Push and open a PR against `main`

### Commit message format

```
<type>: <short description>

Types: feat, fix, docs, refactor, test, chore
```

Examples:
- `feat: add AWS S3 backend`
- `fix: handle bucket already exists error`
- `docs: update Quick Start guide`
- `test: add unit tests for minio client`

---

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep functions small and focused
- Add comments only where logic is non-obvious
- Return errors — don't panic in driver code
- Never log credentials, access keys, or secret values

---

## Areas Most Needed

These are the highest-priority contributions right now:

| Area | Description |
|---|---|
| AWS S3 backend | Implement `internal/aws/client.go` |
| Ceph RGW backend | Implement `internal/ceph/client.go` |
| Credential rotation | Add rotation logic in driver `GrantBucketAccess` |
| Helm chart | Package operator for Helm installation |
| End-to-end tests | Tests against a real MinIO instance |
| Prometheus metrics | Expose bucket/access metrics |

---

## Questions?

Open a [GitHub Discussion](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/discussions) or file an [Issue](https://github.com/DevangRadadiya/k8s-s3-bucket-operator/issues).

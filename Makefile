IMG ?= ghcr.io/devangradadiya/k8s-s3-bucket-operator:latest

.PHONY: all build test lint docker-build docker-push deploy deploy-kustomize undeploy

all: build

## Build the binary locally
build:
	go build -o bin/manager ./cmd/main.go

## Run unit tests
test:
	go test ./... -v -cover

## Run linter
lint:
	go vet ./...

## Build the Docker image
docker-build:
	docker build -t $(IMG) .

## Push the Docker image
docker-push:
	docker push $(IMG)

## Install CRDs and deploy the custom operator
deploy:
	kubectl apply -f deploy/objectstorage.k8s.io_bucketclasses.yaml
	kubectl apply -f deploy/objectstorage.k8s.io_bucketclaims.yaml
	kubectl apply -f deploy/operator.yaml

## Same as deploy, via Kustomize (kubectl built-in)
deploy-kustomize:
	kubectl apply -k deploy/

## Remove the operator deployment
undeploy:
	kubectl delete -f deploy/operator.yaml --ignore-not-found
	kubectl delete -f deploy/objectstorage.k8s.io_bucketclaims.yaml --ignore-not-found
	kubectl delete -f deploy/objectstorage.k8s.io_bucketclasses.yaml --ignore-not-found

## Apply sample BucketClass and BucketClaim
samples:
	kubectl apply -f config/samples/bucketclass.yaml
	kubectl apply -f config/samples/bucketclaim.yaml

## Remove sample resources
samples-clean:
	kubectl delete -f config/samples/bucketclaim.yaml --ignore-not-found
	kubectl delete -f config/samples/bucketclass.yaml --ignore-not-found

## OpenShift deploy
deploy-openshift:
	kubectl apply -f deploy/objectstorage.k8s.io_bucketclasses.yaml
	kubectl apply -f deploy/objectstorage.k8s.io_bucketclaims.yaml
	kubectl apply -f deploy/openshift/scc.yaml
	kubectl apply -f deploy/openshift/operator.yaml

## Tidy Go modules
tidy:
	go mod tidy

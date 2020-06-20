
# Image URL to use all building/pushing image targets
IMG ?= controller:latest
GH_IMG ?= github-webhook:latest
# Produce CRDs that work only after 1.16
CRD_OPTIONS ?= "crd:crdVersions=v1"

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

all: manager github-webhook

# Run tests
test: generate fmt vet manifests
	go test ./... -coverprofile cover.out

# github-Webhook
github-webhook: generate fmt vet
	go build -o bin/github-webhook ./cmd/github-webhook

# Build manager binary
manager: generate fmt vet
	go build -o bin/manager ./cmd/manager

# Run against the configured Kubernetes cluster in ~/.kube/config
run: generate fmt vet manifests
	go run ./cmd/manager

# Install CRDs into a cluster
install: manifests
	kustomize build config/crd | kubectl apply -f -

# Uninstall CRDs from a cluster
uninstall: manifests
	kustomize build config/crd | kubectl delete -f -

# Deploy controller in the configured Kubernetes cluster in ~/.kube/config
rawdeploy: manifests
	cd config/manager && kustomize edit set image controller=${IMG} && kustomize edit set image github-webhook=${GH_IMG}
	cp .env id_rsa config/github-webhook && cp .env id_rsa config/manager
	kustomize build config/default | kubectl apply -f -
	rm config/{github-webhook,manager}/{.env,id_rsa}

deploy: install rawdeploy
	kubectl rollout restart -n properator-system deployment/properator-github-webhook
	kubectl rollout restart -n properator-system deployment/properator-controller-manager

undeploy: manifests
	kustomize build config/default | kubectl delete -f -

# Generate manifests e.g. CRD, RBAC etc.
manifests: controller-gen
	$(CONTROLLER_GEN) $(CRD_OPTIONS) webhook paths="./api/..." output:crd:artifacts:config=config/crd/bases
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=manager paths="./pkg/controllers/..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac
	$(CONTROLLER_GEN) $(CRD_OPTIONS) rbac:roleName=github-webhook paths="./pkg/githubwebhook/..." output:crd:artifacts:config=config/crd/bases output:rbac:artifacts:config=config/rbac/github-webhook

# Run go fmt against code
fmt:
	go fmt ./...

# Run go vet against code
vet:
	go vet ./...

# Generate code
generate: controller-gen
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

# Build the docker image
docker-build: test docker-build-manager docker-build-github-webhook

docker-build-manager:
	docker build . -t ${IMG} --build-arg=target=manager

docker-build-github-webhook:
	docker build . -t ${GH_IMG} --build-arg=target=github-webhook

# Push the docker image
docker-push:
	docker push ${IMG}
	docker push ${GMIMG}

listen-github-webhook:
	./hack/listen.sh

# find or download controller-gen
# download controller-gen if necessary
controller-gen:
ifeq (, $(shell which controller-gen))
	@{ \
	set -e ;\
	CONTROLLER_GEN_TMP_DIR=$$(mktemp -d) ;\
	cd $$CONTROLLER_GEN_TMP_DIR ;\
	go mod init tmp ;\
	go get sigs.k8s.io/controller-tools/cmd/controller-gen@v0.2.5 ;\
	rm -rf $$CONTROLLER_GEN_TMP_DIR ;\
	}
CONTROLLER_GEN=$(GOBIN)/controller-gen
else
CONTROLLER_GEN=$(shell which controller-gen)
endif

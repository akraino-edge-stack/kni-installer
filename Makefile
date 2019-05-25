GOPATH=$(shell pwd)/vendor:$(shell pwd):"${HOME}/go"
GOBIN=$(shell pwd)/bin
GOFILES=$(wildcard *.go)
GONAME="kni-edge-installer"

BUILDDIR = $(shell pwd)/build
INSTALLER_GIT_REPO = github.com/openshift/installer
RHCOS_VERSION = "maipo"
export PATH:=${HOME}/go/bin:${PATH}

ifndef INSTALLER_PATH
override INSTALLER_PATH = https://github.com/openshift/installer/releases/download/v0.16.1/openshift-install-linux-amd64
endif

ifndef INSTALLER_GIT_TAG
override INSTALLER_GIT_TAG = "v0.16.1"
endif

ifndef MASTER_MEMORY_MB
override MASTER_MEMORY_MB = "11192"
endif

ifndef RELEASES_URL
override RELEASES_URL = "https://releases-rhcos.svc.ci.openshift.org/storage/releases/"
endif

ifndef SSH_KEY_PATH
override SSH_KEY_PATH = "${HOME}/.ssh/id_rsa"
endif

ifndef CLUSTER_CREDENTIALS
override CLUSTER_CREDENTIALS="$(shell pwd)/build/auth/kubeconfig"
endif

all: watch

binary:
	@echo
	@echo "Building installer binary"
	@./bin/$(GONAME) binary --build_path ${BUILDDIR} --installer_repository ${INSTALLER_GIT_REPO} --installer_tag ${INSTALLER_GIT_TAG}

build:
	@echo "Building kni-edge-installer with $(GOPATH) to ./bin"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o bin/$(GONAME) $(GOFILES)

clean:
	@echo "Destroying previous cluster"
	@./bin/$(GONAME) clean --build_path $(BUILDDIR)

dependencies:
	@echo "Installing dependencies"
	@./utils/install_dependencies.sh

deploy: dependencies
	@echo "Launching cluster deployment bin/$(GONAME)"
	@./bin/$(GONAME) generate --installer_path $(INSTALLER_PATH) --build_path $(BUILDDIR) --base_repository $(BASE_REPO) --base_path $(BASE_PATH) --secrets_repository $(CREDENTIALS) --site_repository $(SITE_REPO) --settings_path $(SETTINGS_PATH) --master_memory_mb $(MASTER_MEMORY_MB) --ssh_key_path $(SSH_KEY_PATH)
	$(MAKE) workloads

workloads:
	@./bin/$(GONAME) workloads --site_repository $(SITE_REPO) --cluster_credentials $(CLUSTER_CREDENTIALS)

images:
	@echo "Launching image generation"
	@./bin/$(GONAME) images --build_path $(BUILDDIR) --version $(RHCOS_VERSION) --releases_url $(RELEASES_URL)

help:
	@echo "Please use \`make <target>' where <target> is one of"
	@echo "  binary to generate a new openshift-install binary"
	@echo "  build to produce the installer binary"
	@echo "  clean to destroy a previously created cluster and remove build contents"
	@echo "  deploy CREDENTIALS=<github_secret_repo> BASE_REPO=<github_manifests_repo> BASE_PATH=<subpath_on_manifests_repo> SITE_REPO=<github_site_repo> SETTINGS_PATH=<subpath_on_site_repo> SSH_KEY_PATH=<path_to_id_rsa>"
	@echo "  images to download baremetal images"

.PHONY: build get install run watch start stop restart clean

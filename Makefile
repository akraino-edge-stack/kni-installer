GOPATH=$(shell pwd)/vendor:$(shell pwd):"${HOME}/go"
GOBIN=$(shell pwd)/bin
GOFILES=$(wildcard *.go)
GONAME="knictl"

BUILDDIR = $(shell pwd)/build
INSTALLER_GIT_REPO = github.com/openshift/installer
RHCOS_VERSION = "maipo"
export PATH:=${HOME}/go/bin:${PATH}

ifndef INSTALLER_PATH
override INSTALLER_PATH = https://github.com/openshift/installer/releases/download/v0.16.1/openshift-install-linux-amd64
endif

export INSTALLER_GIT_TAG
ifndef INSTALLER_GIT_TAG
override INSTALLER_GIT_TAG = "v0.16.1"
endif

ifndef MASTER_MEMORY_MB
override MASTER_MEMORY_MB = "16384"
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
	echo export INSTALLER_GIT_TAG=${INSTALLER_GIT_TAG} > /tmp/ocp_installer_version
	@./$(GONAME) binary --installer_repository ${INSTALLER_GIT_REPO} --installer_tag ${INSTALLER_GIT_TAG}

build:
	@echo "Building knictl with $(GOPATH) to knictl.tar.gz"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o $(GONAME) $(GOFILES)
	tar -czvf knictl.tar.gz $(GONAME) plugins utils

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
	@./bin/$(GONAME) workloads --site_repository $(SITE_REPO) --cluster_credentials $(CLUSTER_CREDENTIALS) --workload_type customizations
	@./bin/$(GONAME) workloads --site_repository $(SITE_REPO) --cluster_credentials $(CLUSTER_CREDENTIALS) --workload_type workloads

help:
	@echo "Please use \`make <target>' where <target> is one of"
	@echo "  binary to generate a new openshift-install binary"
	@echo "  build to produce the installer binary"
	@echo "  clean to destroy a previously created cluster and remove build contents"
	@echo "  deploy CREDENTIALS=<github_secret_repo> BASE_REPO=<github_manifests_repo> BASE_PATH=<subpath_on_manifests_repo> SITE_REPO=<github_site_repo> SETTINGS_PATH=<subpath_on_site_repo> SSH_KEY_PATH=<path_to_id_rsa>"

.PHONY: build get install run watch start stop restart clean

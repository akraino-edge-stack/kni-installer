GOPATH=$(shell pwd)/vendor:$(shell pwd):"${HOME}/go"
GOBIN=$(shell pwd)/bin
GOFILES=$(wildcard *.go)
GONAME="kni-edge-installer"

BUILDDIR = $(shell pwd)/build
INSTALLER_GIT_REPO = github.com/openshift/installer

ifndef INSTALLER_PATH
override INSTALLER_PATH = https://github.com/openshift/installer/releases/download/v0.14.0/openshift-install-linux-amd64
endif

ifndef INSTALLER_GIT_TAG
override INSTALLER_GIT_TAG = "v0.14.0"
endif

all: watch

binary:
	@echo
	@echo "Building installer binary"
	@./bin/$(GONAME) binary --build_path ${BUILDDIR} --installer_repository ${INSTALLER_GIT_REPO} --installer_tag ${INSTALLER_GIT_TAG}

build:
	@echo "Building kni-edge-installer with $(GOPATH) to ./bin"
	@GOPATH=$(GOPATH) GOBIN=$(GOBIN) go build -o bin/$(GONAME) $(GOFILES)

deploy:
	@echo "Launching cluster deployment bin/$(GONAME)"
	@./bin/$(GONAME) generate --installer_path $(INSTALLER_PATH) --build_path $(BUILDDIR) --base_repository $(BASE_REPO) --base_path $(BASE_PATH) --secrets_repository $(CREDENTIALS) --site_repository $(SITE_REPO) --settings_path $(SETTINGS_PATH)

clean:
	@echo "Destroying previous cluster"
	@./bin/$(GONAME) clean --build_path $(BUILDDIR)

help:
	@echo "Please use \`make <target>' where <target> is one of"
	@echo "  binary to generate a new openshift-install binary"
	@echo "  build to produce the installer binary"
	@echo "  clean to destroy a previously created cluster and remove build contents"
	@echo "  deploy CREDENTIALS=<github_secret_repo> SETTINGS=<path_to_site_settings> BASE_REPO=<github_manifests_repo> BASE_PATH=<subpath_on_manifests_repo>"

.PHONY: build get install run watch start stop restart clean

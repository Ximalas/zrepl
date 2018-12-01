.PHONY: generate build test vet cover release docs docs-clean clean vendordeps
.DEFAULT_GOAL := build

ROOT := github.com/zrepl/zrepl
SUBPKGS += client
SUBPKGS += config
SUBPKGS += daemon
SUBPKGS += daemon/filters
SUBPKGS += daemon/job
SUBPKGS += daemon/logging
SUBPKGS += daemon/nethelpers
SUBPKGS += daemon/pruner
SUBPKGS += daemon/snapper
SUBPKGS += daemon/streamrpcconfig
SUBPKGS += daemon/transport
SUBPKGS += daemon/transport/connecter
SUBPKGS += daemon/transport/serve
SUBPKGS += endpoint
SUBPKGS += logger
SUBPKGS += pruning
SUBPKGS += pruning/retentiongrid
SUBPKGS += replication
SUBPKGS += replication/fsrep
SUBPKGS += replication/pdu
SUBPKGS += replication/internal/diff
SUBPKGS += tlsconf
SUBPKGS += util
SUBPKGS += util/socketpair
SUBPKGS += util/watchdog
SUBPKGS += util/envconst
SUBPKGS += version
SUBPKGS += zfs

_TESTPKGS := $(ROOT) $(foreach p,$(SUBPKGS),$(ROOT)/$(p))

ARTIFACTDIR := artifacts

ifdef ZREPL_VERSION
    _ZREPL_VERSION := $(ZREPL_VERSION)
endif
ifndef _ZREPL_VERSION
    _ZREPL_VERSION := $(shell git describe --dirty 2>/dev/null || echo "ZREPL_BUILD_INVALID_VERSION" )
    ifeq ($(_ZREPL_VERSION),ZREPL_BUILD_INVALID_VERSION) # can't use .SHELLSTATUS because Debian Stretch is still on gmake 4.1
        $(error cannot infer variable ZREPL_VERSION using git and variable is not overriden by make invocation)
    endif
endif
GO_LDFLAGS := "-X github.com/zrepl/zrepl/version.zreplVersion=$(_ZREPL_VERSION)"

GO_BUILD := go build -ldflags $(GO_LDFLAGS)

RELEASE_BINS := $(ARTIFACTDIR)/zrepl-freebsd-amd64
RELEASE_BINS += $(ARTIFACTDIR)/zrepl-linux-amd64
RELEASE_BINS += $(ARTIFACTDIR)/zrepl-darwin-amd64

RELEASE_NOARCH := $(ARTIFACTDIR)/zrepl-noarch.tar
THIS_PLATFORM_RELEASE_BIN := $(shell bash -c 'source <(go env) && echo "zrepl-$${GOOS}-$${GOARCH}"' )

vendordeps:
	dep ensure -v -vendor-only

generate: #not part of the build, must do that manually
	protoc -I=replication/pdu --go_out=replication/pdu replication/pdu/pdu.proto
	@for pkg in $(_TESTPKGS); do\
		go generate "$$pkg" || exit 1; \
	done;

build:
	@echo "INFO: In case of missing dependencies, run 'make vendordeps'"
	$(GO_BUILD) -o "$(ARTIFACTDIR)/zrepl"

test:
	@for pkg in $(_TESTPKGS); do \
		echo "Testing $$pkg"; \
		go test "$$pkg" || exit 1; \
	done;

vet:
	@for pkg in $(_TESTPKGS); do \
		echo "Vetting $$pkg"; \
		go vet "$$pkg" || exit 1; \
	done;

cover: artifacts
	@for pkg in $(_TESTPKGS); do \
		profile="$(ARTIFACTDIR)/cover-$$(basename $$pkg).out"; \
		go test -coverprofile "$$profile" $$pkg || exit 1; \
		if [ -f "$$profile" ]; then \
   			go tool cover -html="$$profile" -o "$${profile}.html" || exit 2; \
		fi; \
	done;

$(ARTIFACTDIR):
	mkdir -p "$@"

$(ARTIFACTDIR)/docs: $(ARTIFACTDIR)
	mkdir -p "$@"

$(ARTIFACTDIR)/bash_completion: $(RELEASE_BINS)
	artifacts/$(THIS_PLATFORM_RELEASE_BIN) bashcomp "$@"

$(ARTIFACTDIR)/go_version.txt:
	go version > $@

docs: $(ARTIFACTDIR)/docs
	make -C docs \
		html \
		BUILDDIR=../artifacts/docs \

docs-clean:
	make -C docs \
		clean \
		BUILDDIR=../artifacts/docs


.PHONY: $(RELEASE_BINS)
# TODO: two wildcards possible
$(RELEASE_BINS): $(ARTIFACTDIR)/zrepl-%-amd64: generate $(ARTIFACTDIR) vet test
	@echo "INFO: In case of missing dependencies, run 'make vendordeps'"
	GOOS=$* GOARCH=amd64   $(GO_BUILD) -o "$(ARTIFACTDIR)/zrepl-$*-amd64"

$(RELEASE_NOARCH): docs $(ARTIFACTDIR)/bash_completion $(ARTIFACTDIR)/go_version.txt
	tar --mtime='1970-01-01' --sort=name \
		--transform 's/$(ARTIFACTDIR)/zrepl-$(_ZREPL_VERSION)-noarch/' \
		-acf $@ \
		$(ARTIFACTDIR)/docs/html \
		$(ARTIFACTDIR)/bash_completion \
		$(ARTIFACTDIR)/go_version.txt

release: $(RELEASE_BINS) $(RELEASE_NOARCH)
	rm -rf "$(ARTIFACTDIR)/release"
	mkdir -p "$(ARTIFACTDIR)/release"
	cp $^ "$(ARTIFACTDIR)/release"
	cd "$(ARTIFACTDIR)/release" && sha512sum $$(ls | sort) > sha512sum.txt
	@# note that we use ZREPL_VERSION and not _ZREPL_VERSION because we want to detect the override
	@if git describe --dirty 2>/dev/null | grep dirty >/dev/null; then \
        echo '[INFO] either git reports checkout is dirty or git is not installed or this is not a git checkout'; \
		if [ "$(ZREPL_VERSION)" = "" ]; then \
			echo '[WARN] git checkout is dirty and make variable ZREPL_VERSION was not used to override'; \
			exit 1; \
		fi; \
	fi;

clean: docs-clean
	rm -rf "$(ARTIFACTDIR)"

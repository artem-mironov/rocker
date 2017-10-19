VERSION ?= $(shell cat VERSION)

GITCOMMIT = $(shell git rev-parse HEAD 2>/dev/null)
GITBRANCH = $(shell git rev-parse --abbrev-ref HEAD 2>/dev/null)
BUILDTIME := $(shell TZ=GMT date "+%Y-%m-%d_%H:%M_GMT")

SRCS = $(shell find . -name '*.go' | grep -v '^./vendor/')
PKGS := $(foreach pkg, $(sort $(dir $(SRCS))), $(pkg))

TESTARGS ?=

export GO15VENDOREXPERIMENT := 1

GOARCHs = amd64
GOOSs = darwin linux

default:
	go build -v

install:
	go install -v

dist_dir:
	
	@for GOOS in $(GOOSs) ; do \
		for GOARCH in $(GOARCHs) ; do \
			mkdir -p ./dist/$${GOOS}_$${GOARCH} ; \
		done \
	done


cross: dist_dir

	@for GOOS in $(GOOSs) ; do \
		for GOARCH in $(GOARCHs) ; do \
			docker run --rm -ti -v $(shell pwd):/go/src/github.com/grammarly/rocker \
				-e GOOS=$${GOOS} -e GOARCH=$${GOARCH} \
				-w /go/src/github.com/grammarly/rocker \
				golang:1.8 go build \
				-ldflags "-X main.Version=$(VERSION) -X main.GitCommit=$(GITCOMMIT) -X main.GitBranch=$(GITBRANCH) -X main.BuildTime=$(BUILDTIME)" \
				-v -o ./dist/$${GOOS}_$${GOARCH}/rocker ; \
		done \
	done


cross_tars: cross

	@for GOOS in $(GOOSs) ; do \
		for GOARCH in $(GOARCHs) ; do \
			COPYFILE_DISABLE=1 tar -zcvf ./dist/rocker_$${GOOS}_$${GOARCH}.tar.gz -C dist/$${GOOS}_$${GOARCH} rocker ; \
		done \
	done
	
clean:
	rm -Rf dist

testdeps:
	@ go get github.com/GeertJohan/fgt

fmtcheck:
	$(foreach file,$(SRCS),gofmt $(file) | diff -u $(file) - || exit;)

lint:
	@ go get -u github.com/golang/lint/golint
	$(foreach file,$(SRCS),fgt golint $(file) || exit;)

vet:
	$(foreach pkg,$(PKGS),fgt go vet $(pkg) || exit;)

gocyclo:
	@ go get github.com/fzipp/gocyclo
	gocyclo -over 25 ./src

test: testdeps fmtcheck vet lint
	go test ./src/... $(TESTARGS)

test_integration:
	go test -v ./test/... --args -verbosity=0

.PHONY: clean test fmtcheck lint vet gocyclo default

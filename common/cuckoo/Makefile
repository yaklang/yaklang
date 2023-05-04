ROOT := $(CURDIR)
GOPKGS = \
		golang.org/x/tools/cmd/cover \
		github.com/golang/lint/golint \
		github.com/golang/dep/cmd/dep

default: test

deps:
	@go get -u -v $(GOPKGS)
	@dep ensure

lint:
	@echo "[Lint] running golint"
	@golint

vet:
	@echo "[Vet] running go vet"
	@go vet

ci: deps vet lint test

test:
	@echo "[Test] running tests"
	@if [ "$(CI)" ]; then goveralls -service=travis-ci; else go test -v -cover; fi

.PHONY: default golint test vet deps

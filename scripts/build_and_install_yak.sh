#!/usr/bin/env bash

go build -ldflags "-X 'main.goVersion=$(go version)' -X 'main.gitHash=$(git show -s --format=%H)' -X 'main.buildTime=$(git show -s --format=%cd)' -X 'main.yakVersion=$(git describe --tag)'" -o yak common/yak/cmd/yak.go && sudo mv ./yak /usr/local/bin

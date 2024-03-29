#!/bin/zsh

find common -name '*_test.go' -exec rm -f {} +
rm -rf common/vulinbox

go build -ldflags "-s -w" -o yak common/yak/cmd/yak.go
ls -lh | grep yak
rm yak

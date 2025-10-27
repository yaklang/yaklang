#!/bin/sh
clear
clear

SKIP_SYNC_BUILD_IN_AI_TOOL=1 go run common/yak/cmd/yak.go grpc

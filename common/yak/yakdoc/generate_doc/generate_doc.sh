#!/bin/sh
SHELL_FOLDER=$(dirname $(readlink -f "$0"))
cd $SHELL_FOLDER
go run -gcflags=all="-N -l" $SHELL_FOLDER/generate_doc.go $SHELL_FOLDER/../doc/doc.gob.gzip
#!/bin/sh
SHELL_FOLDER=$(dirname $(readlink -f "$0"))
cd $SHELL_FOLDER
go run $SHELL_FOLDER/generate_doc.go $SHELL_FOLDER/../doc/doc.gob.gzip
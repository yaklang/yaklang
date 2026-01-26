#!/bin/bash
set -e

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

# Build the runtime from C source
cd "$SCRIPT_DIR/runtime_c"
gcc -c yak_runtime.c -o ../yak_runtime.o
ar rcs ../libyak.a ../yak_runtime.o
cd ..

echo "Built libyak.a (C Runtime) and yak_runtime.o"

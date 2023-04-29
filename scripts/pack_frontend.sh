#!/usr/bin/env bash

set -e

mkdir -p www
cd frontend
rm -rf ./build
yarn build
cp -r build/ ../www/fe


cd ../
cp ./scripts/install_pack_frontend.sh www
makeself  www build/frontend.package "frontend package"  ./install_pack_frontend.sh

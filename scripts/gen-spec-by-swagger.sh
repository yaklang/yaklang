#!/usr/bin/env bash 
set -e

rm -rf server/web/gen/*
#swagger 0.29.0
swagger generate server -f swagger.yml -t server/web/gen --regenerate-configureapi -P models.Principle
#
# npx @manifoldco/swagger-to-ts@1 swagger.yml --wrapper "export declare namespace Palm" --output ./frontend/src/gen/schema.d.ts
# ------
# 如果没有 swagger-to-ts 通过  npm install -g @manifoldco/swagger-to-ts@1.7.1
#
swagger-to-ts swagger.yml --wrapper "export declare namespace Palm" --output ./frontend/src/gen/schema.d.ts

# ./frontend/src/gen/schema.d.ts
rm ./wizard/src/gen/schema.d.ts
cp ./frontend/src/gen/schema.d.ts ./wizard/src/gen/schema.d.ts


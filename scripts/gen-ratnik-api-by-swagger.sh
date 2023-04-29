#!/usr/bin/env bash
set -e

rm -rf ratnik/rwebgen/*
swagger generate server -f ratnik/ratnik.swagger.yaml -t ratnik/rwebgen --regenerate-configureapi -P models.Principle
npx @manifoldco/swagger-to-ts@1 swagger.yml --wrapper "export declare namespace Ratnik" --output ./wizard/src/ratnik/schema.d.ts

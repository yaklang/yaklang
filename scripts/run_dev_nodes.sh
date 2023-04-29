#!/usr/bin/env bash

docker-compose -f scripts/docker-compose.run_node.yml down
docker-compose -f scripts/docker-compose.run_node.yml up

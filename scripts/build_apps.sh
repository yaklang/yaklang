#!/usr/bin/env bash

# 这个应该在项目根目录下执行, 可以在根目录 build 文件夹下编译出可以执行的 linux 环境文件
docker-compose -f scripts/docker-compose.builder.yml up
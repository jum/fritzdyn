#!/bin/sh
DOCKER_PREFIX=jumager/fritzdyn
branch=$(basename $(git rev-parse --abbrev-ref HEAD))
docker buildx build --platform linux/arm64,linux/amd64 -t ${DOCKER_PREFIX}:${branch} -f Dockerfile --push .

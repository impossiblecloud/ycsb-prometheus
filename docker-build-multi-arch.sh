#!/bin/bash

IMAGE="adgit/ycsb-prometheus"
TAG="20250521-v1"

docker buildx build \
  --no-cache \
  --push \
  --platform linux/arm64,linux/amd64 \
  --tag ${IMAGE}:${TAG} .

#!/usr/bin/env bash

IMAGE_NAME=appdynamics/cluster-agent
if [ "x$1" != "x" ]; then
  IMAGE_NAME=$1
fi
docker build -t ${IMAGE_NAME}:$2 --no-cache -f build/Dockerfile  . \
&& docker push ${IMAGE_NAME}:$2 
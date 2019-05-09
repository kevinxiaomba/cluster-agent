#!/usr/bin/env bash

IMAGE_NAME=appdynamics/cluster-agent
DOCKERFILE=build/Dockerfile

if [ "x$1" != "x" ]; then
  IMAGE_NAME=$1
fi

if [ "x$3" != "x" ]; then
  DOCKERFILE=$3
fi

docker build -t ${IMAGE_NAME}:$2 --no-cache -f ${DOCKERFILE}  . \
&& docker push ${IMAGE_NAME}:$2 
#!/bin/bash

set -e

cd "$(dirname $0)/../../.."

docker run \
  -v `pwd`:/go/src/github.com/kubermatic/kubermatic golang:1.14.2 \
  /bin/sh -c 'cd /go/src/github.com/kubermatic/kubermatic/api && KUBERMATIC_EDITION=ee make test'

#!/bin/bash

set -e

cd "$(dirname $0)/../../.."

docker run \
  -v `pwd`:/go/src/github.com/kubermatic/kubermatic golangci/golangci-lint:v1.23.8 \
  /bin/sh -c 'cd /go/src/github.com/kubermatic/kubermatic/api && KUBERMATIC_EDITION=ee GOGC=5 make check'

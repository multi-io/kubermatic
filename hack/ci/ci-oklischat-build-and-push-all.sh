#!/bin/bash

set -e

repo="$1"
tag="$2"

if [[ -z "$tag" ]]; then
  echo "no tag specified; not building and pushing an image."
  exit 0
fi

cd "$(dirname $0)/../../.."

# TODO image name copied from .prow.yaml -- need to update this manually when necessary
docker run -v /var/run/docker.sock:/var/run/docker.sock -v "`pwd`:/go/src/github.com/kubermatic/kubermatic" --entrypoint=/bin/bash quay.io/kubermatic/go-docker:14.2-1806-0 -c "
  set -e
  docker login -u '$DOCKER_USERNAME' -p '$DOCKER_PASSWORD';
  DOCKER_REPO=oklischat KUBERMATIC_EDITION=ee /go/src/github.com/kubermatic/kubermatic/api/hack/push_image.sh $tag;
"

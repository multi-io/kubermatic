#!/bin/bash

set -e

repo="$1"

if [[ -z "$repo" ]]; then
  echo "usage: $0 <dockerrepo> [GITTAG KUBERMATICCOMMIT]" >&2
  exit 1
fi

gittag="${2:-$(basename $(git describe --tags --always))}"
kubermaticcommit="${3:-$(git rev-parse HEAD)}"

cd "$(dirname $0)/.."

echo "building $repo $gittag $kubermaticcommit"

# TODO image name copied from .prow.yaml -- need to update this manually when necessary
docker run -v /var/run/docker.sock:/var/run/docker.sock -v "`pwd`:/go/src/github.com/kubermatic/kubermatic" -v "$HOME/.docker:/root/.docker" --entrypoint=/bin/bash quay.io/kubermatic/go-docker:14.2-1806-0 -c "
  set -e
  DOCKER_REPO=$repo KUBERMATIC_EDITION=ee /go/src/github.com/kubermatic/kubermatic/hack/push_image.sh $gittag $kubermaticcommit;
"

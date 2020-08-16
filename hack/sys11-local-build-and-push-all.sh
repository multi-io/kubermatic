#!/bin/bash

set -e

repo="$1"

if [[ -z "$repo" ]]; then
  echo "usage: $0 <dockerrepo> [GITTAG KUBERMATICCOMMIT]" >&2
  exit 1
fi

gittag="${2:-$(basename $(git describe --tags --always))}"
kubermaticcommit="${3:-$(git rev-parse HEAD)}"

if [[ -n "$DEBUG" ]]; then
  gittag="debug-$gittag"
  kubermaticcommit="debug-$kubermaticcommit"
  export GOTOOLFLAGS_EXTRA="-gcflags='all=-N -l'"
  export LDFLAGS_EXTRA=" "   # must not be empty string or it would no longer be defined in the docker run script
fi

cd "$(dirname $0)/.."

echo "building $repo $gittag $kubermaticcommit"

# TODO image name copied from .prow.yaml -- need to update this manually when necessary
docker run -e GOTOOLFLAGS_EXTRA -e LDFLAGS_EXTRA -v /var/run/docker.sock:/var/run/docker.sock -v "`pwd`:/go/src/github.com/kubermatic/kubermatic" -v "$HOME/.docker:/root/.docker" --entrypoint=/bin/bash quay.io/kubermatic/go-docker:14.2-1806-0 -c "
  set -e
  DOCKER_REPO=$repo KUBERMATIC_EDITION=ee GITTAG=$gittag KUBERMATICCOMMIT=$kubermaticcommit /go/src/github.com/kubermatic/kubermatic/hack/push_image.sh $gittag $kubermaticcommit;
"

#!/bin/bash

set -e

usage() {
    cat >&2 <<-EOS
usage: `basename $0` [-l] <dockerrepo> [GITTAG KUBERMATICCOMMIT]"

Builds all Kubermatic images the same way Kubermatic's CI does (essentially
by running hack/release-docker-images.sh in a docker container) and pushes
them to <dockerrepo>. If -l is passed, a docker login to that repo is
performed first -- otherwise ~/.docker will be mounted into the build container
and must contain working credentials for <dockerrepo> so pushing works (which
might not work e.g. on a Mac if the credentials are stored in the Mac Keychain).

The images will be tagged with tag names derived from the checkout out commit
unless you override that with the optional [GITTAG KUBERMATICCOMMIT] arguments.
If you want to use kubermatic-operator, you can also set \$UIDOCKERTAG to the
dashboard tag that will be baked into the built operator binary (the operator
will then use that tag when setting the image property of the dashboard deployment
it creates).

You can optionally set \$SRC_DIR to point to a different kubermatic checkout to
use. This can also be a recent upstream kubermatic version. You can set
\$DEBUG=1 to build binaries with debugging symbols (the image tags will be
prefixed with "debug-" in that case).
EOS
    exit 1
}

if [[ -n "$SRC_DIR" ]]; then
  cd "$SRC_DIR/hack"
fi

while getopts 'l' OPTNAME; do
case "$OPTNAME" in
  "l")
    mustlogin=1 ;;
  "?")
    usage ;;
esac
done

shift $(($OPTIND - 1))

repo="$1"

if [[ -z "$repo" ]]; then
  usage
fi

GITTAG="${2:-$(basename $(git describe --tags --always))}"
KUBERMATICCOMMIT="${3:-$(git rev-parse HEAD)}"

# GITTAG and KUBERMATICCOMMIT to be baked into kubermatic-operator for use when rolling out kubermatic api and controller components,
# and into kubermatic api and controller components for use when rolling out user cluster control plane components
if [[ -n "$DEBUG" ]]; then
  GITTAG="debug-$GITTAG"
  KUBERMATICCOMMIT="debug-$KUBERMATICCOMMIT"
  export GOTOOLFLAGS_EXTRA="-gcflags='all=-N -l'"
  export LDFLAGS_EXTRA=" "   # must not be empty string or it would no longer be defined in the docker run script
fi

# UIDOCKERTAG to be baked into kubermatic-operator binary for use when rolling out kubermatic dashboard
if [[ -z "$UIDOCKERTAG" ]]; then
  UIDOCKERTAG=NA
fi

export KUBERMATICCOMMIT
export GITTAG
export UIDOCKERTAG

cd ..

echo "building $repo $GITTAG $KUBERMATICCOMMIT"

if [[ -n "$mustlogin" ]]; then
  docker_login_opts="-t -i"
  if grep -q / <<<"$repo"; then
    reposerver=$(cut -d/ -f1 <<<"$repo")
  else
    reposerver=docker.io
  fi
else
  docker_login_opts="-v $HOME/.docker:/root/.docker"
fi

if [[ -z "$GO_DOCKER_IMAGE" ]]; then
  GO_DOCKER_IMAGE=$(<"$SRC_DIR/.prow.yaml" yq '.presubmits[] | select(.name=="pre-kubermatic-dependencies") | .spec.containers[0].image' -r)
fi

docker run -e GOTOOLFLAGS_EXTRA -e LDFLAGS_EXTRA -e KUBERMATICCOMMIT -e GITTAG -e UIDOCKERTAG -v /var/run/docker.sock:/var/run/docker.sock -v "`pwd`:/src/kubermatic" ${docker_login_opts} --entrypoint=/bin/bash "${GO_DOCKER_IMAGE}" -c "
  set -e
  if [[ -n '$mustlogin' ]]; then
    echo 'logging into $reposerver'
    docker login $reposerver
  fi
  DOCKER_REPO=$repo KUBERMATIC_EDITION=ce /src/kubermatic/hack/release-docker-images.sh $GITTAG $KUBERMATICCOMMIT;
"

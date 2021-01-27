#!/usr/bin/env bash

# must be run with $KUBECONFIG pointing to a seed cluster.
# Running in a sys11 seed cluster isn't recommended as we're not
# using kubermatic-operator in our installer, so running it in
# a sys11 seed will lack a KubermaticConfiguration resource
# in the seed, and if that was, it would "overwrite" existing
# resources and probably break things.

#set -euo pipefail
set -e

cd $(dirname $0)/..
source hack/lib.sh

: "${SRC_DIR:=$(pwd)}"
: "${SYS11_HACK_DIR:=$(${SRC_DIR}/hack)}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-${LOGNAME}}}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"
if [[ -n "$RUN_IN_SYS11_SEED" ]]; then
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} make -C ${INSTALLER_DIR}/kubermatic values.yaml
fi
: "${DEBUG:="false"}"
: "${KUBERMATICCOMMIT:="$(git rev-parse HEAD)"}"
: "${GITTAG:=$(git describe --tags --always)}"
: "${KUBERMATIC_EDITION:=ee}"
: "${NAMESPACE:=kubermatic}"

export KUBERMATIC_EDITION

if [[ "${TAG_WORKER}" == "true" ]]; then
    WORKER_OPTION="-worker-name=$(tr -cd '[:alnum:]' <<< ${KUBERMATIC_WORKERNAME} | tr '[:upper:]' '[:lower:]')"
else
    WORKER_OPTION=
fi

if [ -z "$NAMESPACE" ]; then
  echo "You must specify a NAMESPACE environment variable to run the operator in."
  echo "Do not use a namespace where another operator is already running."
  echo "Example: NAMESPACE=kubermatic ./hack/run-operator.sh"
  exit 2
fi

echodate "Compiling operator..."

if [[ "${DEBUG}" == "true" ]]; then
    make GOTOOLFLAGS_EXTRA="-gcflags='all=-N -l'" LDFLAGS_EXTRA="" kubermatic-operator
else
    make kubermatic-operator
fi

echo

echodate "Starting operator..."
set -x

if [[ "${DEBUG}" == "true" ]]; then
    dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/kubermatic-operator -- \
      -kubeconfig=$KUBECONFIG \
      -namespace="$NAMESPACE" \
      ${WORKER_OPTION} \
      -log-debug=1 \
      -log-format=Console \
      -logtostderr \
      -v=8

else
    ./_build/kubermatic-operator -- \
      -kubeconfig=$KUBECONFIG \
      -namespace="$NAMESPACE" \
      ${WORKER_OPTION} \
      -log-debug=1 \
      -log-format=Console \
      -logtostderr \
      -v=8

fi

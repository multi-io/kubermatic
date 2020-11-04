#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -x
set -e

: "${DASHBOARD_SRC_DIR:=$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-dashboard}"

cd $(dirname $0)

function cleanup {
  set +e
  MAIN_PID=$(jobs -l|grep run-api.sh|awk '{print $2}')
  # There is no `kill job and all its children` :(
  kill $(pgrep -P $MAIN_PID)
  kill $MAIN_PID
}
trap cleanup EXIT

echo "starting api"
./sys11-run-api.sh &
echo "finished starting api"

echo "Starting dashboard"
"${DASHBOARD_SRC_DIR}/hack/sys11-run-local-dashboard.sh"

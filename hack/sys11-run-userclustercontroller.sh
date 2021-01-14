#!/bin/bash

set -o errexit
set -o pipefail

if [[ -z "$USERCLUSTER" ]]; then
  echo "set \$USERCLUSTER to the cluster id of the user cluster you want to run in"
  exit 1
fi

: "${SRC_DIR:=$(go env GOPATH)/src/k8c.io/kubermatic}"
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"
: "${DEBUG:="false"}"

export KUBECONFIG="${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/clusters/${KUBERMATIC_CLUSTER}/kubeconfig/admin.conf"

# Getting everything we need from the api
ADMIN_KUBECONFIG_RAW="$(kubectl -n "cluster-$USERCLUSTER" get secret admin-kubeconfig -o json)"
CLUSTER_RAW="$(kubectl -n "cluster-$USERCLUSTER" get cluster $(echo $ADMIN_KUBECONFIG_RAW|jq -r '.metadata.namespace'|sed 's/cluster-//') -o json)"
OPENVPN_SERVER_SERVICE_RAW="$(kubectl -n "cluster-$USERCLUSTER" get service openvpn-server -o json )"
USERSSHKEYS_SECRET_RAW="$(kubectl -n "cluster-$USERCLUSTER" get secret usersshkeys -o json)"

KUBECONFIG_USERCLUSTER_CONTROLLER_FILE=$(mktemp)
trap "rm -f $KUBECONFIG_USERCLUSTER_CONTROLLER_FILE" EXIT

echo ${ADMIN_KUBECONFIG_RAW}|jq -r '.data.kubeconfig' |base64 --decode > ${KUBECONFIG_USERCLUSTER_CONTROLLER_FILE}

CLUSTER_VERSION="$(echo $CLUSTER_RAW|jq -r '.spec.version')"
CLUSTER_NAMESPACE="$(echo $ADMIN_KUBECONFIG_RAW|jq -r '.metadata.namespace')"
CLUSTER_URL="$(echo $CLUSTER_RAW | jq -r .address.url)"
OPENVPN_SERVER_NODEPORT="$(echo ${OPENVPN_SERVER_SERVICE_RAW} | jq -r .spec.ports[0].nodePort)"
OWNER_EMAIL="$(echo $CLUSTER_RAW | jq .status.userEmail -r)"

ARGS=""
if echo $CLUSTER_RAW |grep openshift -q; then
  ARGS="-openshift=true"
fi

if echo $CLUSTER_RAW|grep -i aws -q; then
  ARGS="$ARGS -cloud-provider-name=aws"
fi

if [[ "${DEBUG}" == "true" ]]; then
    GOTOOLFLAGS="-v -gcflags='all=-N -l'" make -C ${SRC_DIR} user-cluster-controller-manager
else
    make -C ${SRC_DIR} user-cluster-controller-manager
fi

cd ${SRC_DIR}
if [[ "${DEBUG}" == "true" ]]; then
    dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/user-cluster-controller-manager -- \
        -kubeconfig=${KUBECONFIG_USERCLUSTER_CONTROLLER_FILE} \
        -metrics-listen-address=127.0.0.1:8087 \
        -health-listen-address=127.0.0.1:8088 \
        -namespace=${CLUSTER_NAMESPACE} \
        -openvpn-server-port=${OPENVPN_SERVER_NODEPORT} \
        -cluster-url=${CLUSTER_URL} \
        -version=${CLUSTER_VERSION} \
        -log-debug=true \
        -log-format=Console \
        -logtostderr \
        -v=4 \
        -seed-kubeconfig=${KUBECONFIG} \
        -owner-email=${OWNER_EMAIL} \
        -dns-cluster-ip=10.240.16.10 \
        ${ARGS}
else
    ./_build/user-cluster-controller-manager \
        -kubeconfig=${KUBECONFIG_USERCLUSTER_CONTROLLER_FILE} \
        -metrics-listen-address=127.0.0.1:8087 \
        -health-listen-address=127.0.0.1:8088 \
        -namespace=${CLUSTER_NAMESPACE} \
        -openvpn-server-port=${OPENVPN_SERVER_NODEPORT} \
        -cluster-url=${CLUSTER_URL} \
        -version=${CLUSTER_VERSION} \
        -log-debug=true \
        -log-format=Console \
        -logtostderr \
        -v=4 \
        -seed-kubeconfig=${KUBECONFIG} \
        -owner-email=${OWNER_EMAIL} \
        -dns-cluster-ip=10.240.16.10 \
        ${ARGS}
fi

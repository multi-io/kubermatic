#!/bin/bash

set -o errexit
set -o pipefail

if [[ -z "$USERCLUSTER" ]]; then
  echo "set \$USERCLUSTER to the cluster id of the user cluster you want to run in"
  exit 1
fi

: "${SRC_DIR:=$(go env GOPATH)/src/github.com/kubermatic/kubermatic/api}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-$(uname -n)}}"
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"
: "${DEBUG:="false"}"

export KUBECONFIG="${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/clusters/${KUBERMATIC_CLUSTER}/kubeconfig/admin.conf"

# Getting everything we need from the api
ADMIN_KUBECONFIG_RAW="$(kubectl -n "cluster-$USERCLUSTER" get secret admin-kubeconfig -o json)"
CA_CERT_RAW="$(kubectl -n "cluster-$USERCLUSTER" get secret ca -o json)"
OPENVPN_CA_SECRET_RAW="$(kubectl -n "cluster-$USERCLUSTER" get secret openvpn-ca -o json)"
CLUSTER_RAW="$(kubectl -n "cluster-$USERCLUSTER" get cluster $(echo $ADMIN_KUBECONFIG_RAW|jq -r '.metadata.namespace'|sed 's/cluster-//') -o json)"
OPENVPN_SERVER_SERVICE_RAW="$(kubectl -n "cluster-$USERCLUSTER" get service openvpn-server -o json )"
USERSSHKEYS_SECRET_RAW="$(kubectl -n "cluster-$USERCLUSTER" get secret usersshkeys -o json)"

CA_CERT_FILE=$(mktemp)
CA_CERT_KEY_FILE=$(mktemp)
OPENVPN_CA_CERT_FILE=$(mktemp)
OPENVPN_CA_KEY_FILE=$(mktemp)
KUBECONFIG_USERCLUSTER_CONTROLLER_FILE=$(mktemp)
SSH_KEYS_DIR=$(mktemp -d)
trap "rm -f $CA_CERT_FILE" EXIT
trap "rm -f $CA_CERT_KEY_FILE" EXIT
trap "rm -f $OPENVPN_CA_CERT_FILE" EXIT
trap "rm -f $OPENVPN_CA_KEY_FILE" EXIT
trap "rm -f $KUBECONFIG_USERCLUSTER_CONTROLLER_FILE" EXIT
trap "rm -rf $SSH_KEYS_DIR" EXIT

echo ${CA_CERT_RAW}|jq -r '.data["ca.crt"]'|base64 --decode > ${CA_CERT_FILE}
echo ${CA_CERT_RAW}|jq -r '.data["ca.key"]'|base64 --decode > ${CA_CERT_KEY_FILE}
echo ${OPENVPN_CA_SECRET_RAW}|jq -r '.data["ca.crt"]'|base64 --decode > ${OPENVPN_CA_CERT_FILE}
echo ${OPENVPN_CA_SECRET_RAW}|jq -r '.data["ca.key"]'|base64 --decode > ${OPENVPN_CA_KEY_FILE}
echo ${ADMIN_KUBECONFIG_RAW}|jq -r '.data.kubeconfig' |base64 --decode > ${KUBECONFIG_USERCLUSTER_CONTROLLER_FILE}
mkdir -p "${SSH_KEYS_DIR}/linktarget"
(cd "${SSH_KEYS_DIR}" && ln -s "linktarget" "..data")
echo ${USERSSHKEYS_SECRET_RAW} | jq '.data | to_entries[] | .key + " " + .value' -r | while read k v; do
  echo "$v" | base64 --decode >"${SSH_KEYS_DIR}/linktarget/${k}";
  (cd "${SSH_KEYS_DIR}" && ln -s "..data/$k" "$k")
done

CLUSTER_VERSION="$(echo $CLUSTER_RAW|jq -r '.spec.version')"
CLUSTER_NAMESPACE="$(echo $ADMIN_KUBECONFIG_RAW|jq -r '.metadata.namespace')"
CLUSTER_URL="$(echo $CLUSTER_RAW | jq -r .address.url)"
OPENVPN_SERVER_NODEPORT="$(echo ${OPENVPN_SERVER_SERVICE_RAW} | jq -r .spec.ports[0].nodePort)"

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
        -openvpn-ca-cert-file=${OPENVPN_CA_CERT_FILE} \
        -openvpn-ca-key-file=${OPENVPN_CA_KEY_FILE} \
        -cluster-url=${CLUSTER_URL} \
        -ca-cert=${CA_CERT_FILE} \
        -ca-key=${CA_CERT_KEY_FILE} \
        -user-ssh-keys-dir-path=${SSH_KEYS_DIR} \
        -version=${CLUSTER_VERSION} \
        -log-debug=true \
        -log-format=Console \
        ${ARGS}

else
    ./_build/user-cluster-controller-manager \
        -kubeconfig=${KUBECONFIG_USERCLUSTER_CONTROLLER_FILE} \
        -metrics-listen-address=127.0.0.1:8087 \
        -health-listen-address=127.0.0.1:8088 \
        -namespace=${CLUSTER_NAMESPACE} \
        -openvpn-server-port=${OPENVPN_SERVER_NODEPORT} \
        -openvpn-ca-cert-file=${OPENVPN_CA_CERT_FILE} \
        -openvpn-ca-key-file=${OPENVPN_CA_KEY_FILE} \
        -cluster-url=${CLUSTER_URL} \
        -ca-cert=${CA_CERT_FILE} \
        -ca-key=${CA_CERT_KEY_FILE} \
        -user-ssh-keys-dir-path=${SSH_KEYS_DIR} \
        -version=${CLUSTER_VERSION} \
        -log-debug=true \
        -log-format=Console \
        ${ARGS}
fi

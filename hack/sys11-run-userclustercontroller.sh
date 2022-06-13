#!/bin/bash

set -o errexit
set -o pipefail

cd "$(dirname $0)"

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
CLUSTER_RAW="$(kubectl get cluster $USERCLUSTER -o json)"
OPENVPN_SERVER_SERVICE_RAW="$(kubectl -n "cluster-$USERCLUSTER" get service openvpn-server -o json )"

tmpdir="$(mktemp -d)"
trap "rm -rf $tmpdir" EXIT

KUBECONFIG_USERCLUSTER_CONTROLLER_FILE="${tmpdir}/kubeconfig-usercluster-controller"
kubectl -n "cluster-$USERCLUSTER" get secret internal-admin-kubeconfig -o json | jq .data.kubeconfig -r  | base64 --decode >"$KUBECONFIG_USERCLUSTER_CONTROLLER_FILE"
public_api_endpoint="$(kubectl -n "cluster-$USERCLUSTER" get secret admin-kubeconfig -o json | jq .data.kubeconfig -r | base64 --decode | yq '.clusters[0].cluster.server' -r)"
kubectl --kubeconfig "$KUBECONFIG_USERCLUSTER_CONTROLLER_FILE" config set-cluster "$USERCLUSTER" --server="$public_api_endpoint"

CLUSTER_VERSION="$(echo $CLUSTER_RAW|jq -r '.spec.version')"
CLUSTER_URL="$(echo $CLUSTER_RAW | jq -r .address.url)"
OPENVPN_SERVER_NODEPORT="$(echo ${OPENVPN_SERVER_SERVICE_RAW} | jq -r .spec.ports[0].nodePort)"
OWNER_EMAIL="$(echo $CLUSTER_RAW | jq .status.userEmail -r)"

kubectl -n "cluster-$USERCLUSTER" get cm ca-bundle -o json | jq '.data."ca-bundle.pem"' -r >"${tmpdir}/ca-bundle"

SEED_KUBECONFIG="${tmpdir}/seed-kubeconfig"
./kube-serviceaccount-kubeconfig "cluster-$USERCLUSTER" kubermatic-usercluster-controller-manager "$SEED_KUBECONFIG"

ARGS=""
if echo $CLUSTER_RAW |grep openshift -q; then
  ARGS="-openshift=true"
fi

if echo $CLUSTER_RAW|grep -i aws -q; then
  ARGS="$ARGS -cloud-provider-name=aws"
fi

dns_domain="$(echo $CLUSTER_RAW | jq -r .spec.clusterNetwork.dnsDomain)"
if [[ -n "$dns_domain" && "$dns_domain" != "cluster.local" && "$dns_domain" != "null" ]]; then
  ARGS="$ARGS -cluster-dns-domain=$dns_domain"
fi

ARGS="$ARGS -enable-ssh-key-agent=$(echo $CLUSTER_RAW | jq '.spec.enableUserSSHKeyAgent // true')"

if [[ "${DEBUG}" == "true" ]]; then
    GOTOOLFLAGS="-v -gcflags='all=-N -l'" make -C ${SRC_DIR} user-cluster-controller-manager
    RUNCMD="dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/user-cluster-controller-manager --"
else
    make -C ${SRC_DIR} user-cluster-controller-manager
    RUNCMD="./_build/user-cluster-controller-manager"
fi

cd ${SRC_DIR}
$RUNCMD \
  -kubeconfig=${KUBECONFIG_USERCLUSTER_CONTROLLER_FILE} \
  -metrics-listen-address=127.0.0.1:8087 \
  -health-listen-address=127.0.0.1:8088 \
  -namespace="cluster-$USERCLUSTER" \
  -cluster-name=$USERCLUSTER \
  -openvpn-server-port=${OPENVPN_SERVER_NODEPORT} \
  -cluster-url=${CLUSTER_URL} \
  -version=${CLUSTER_VERSION} \
  -log-debug=true \
  -log-format=Console \
  -logtostderr \
  -v=4 \
  -seed-kubeconfig="${SEED_KUBECONFIG}" \
  -owner-email=${OWNER_EMAIL} \
  -dns-cluster-ip=10.240.16.10 \
  -user-cluster-monitoring=false \
  -user-cluster-logging=false \
  -ca-bundle="${tmpdir}/ca-bundle" \
  ${ARGS}

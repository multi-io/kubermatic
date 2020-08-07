#!/bin/bash

set -o errexit
set -o pipefail
set -x

cd "$(dirname $0)"

: "${SRC_DIR:=$(cd ..; pwd)}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-${USERNAME}}}"
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"
: "${RESOURCES_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/clusters/${KUBERMATIC_CLUSTER}/kubermatic/versions}"
: "${CONFIG_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/kubermatic}"
if [[ -z "$SKIP_INSTALLER" ]]; then
KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} make -C ${INSTALLER_DIR}/kubermatic values.yaml
fi
: "${EXTERNAL_URL:=dev.metakube.de}"
: "${DEBUG:="false"}"
: "${KUBERMATICCOMMIT:="$(git rev-parse HEAD)"}"
: "${GITTAG:=$(git describe --tags --always)}"
: "${DYNAMIC_DATACENTERS:="false"}"   # true | false | absent  -- absent meaning pass neither -datacenters= nor -dynamic-datacenters= (2.15+)
: "${IS_KUBERMATIC_UPSTREAM:="false"}"
: "${KUBERMATIC_IMAGE:="docker.io/syseleven/kubermatic"}"
: "${DNATCONTROLLER_IMAGE:="docker.io/syseleven/kubeletdnat-controller"}"
: "${ETCD_LAUNCHER_IMAGE:="docker.io/syseleven/etcd-launcher"}"
: "${S3_ENDPOINT:="s3.cbk.cloud.syseleven.net"}"
: "${S3_BUCKET:="metakube-etcd-backup-dev"}"
: "${S3_SNAPSHOT_DIR:="${INSTALLER_DIR}/snapshots"}"
: "${KUBERMATIC_EDITION:=ee}"

# $GITTAG and $KUBERMATICCOMMIT will be compiled into the binary. $GITTAG will only be used for reporting
# Kubermatic version information via the API; $KUBERMATICCOMMIT will be set as the image tag for
# rolled out user cluster apiserver pod sidecar containers, e.g. the docker.io/syseleven/kubeletdnat-controller image.
# So is must refer to a tag name for which we've built and uploaded kubermatic images.
# The CI will push two images by default, one tagged with $KUBERMATICCOMMIT and one tagged with $GITTAG.
# If you're running with locally committed and not yet pushed changes, you must
# override those variables, e.g. KUBERMATICCOMMIT=v2.13.1-sys11-12 GITTAG=v2.13.1-sys11-12 ... ./sys11-run-controller.sh

if [[ "${IS_KUBERMATIC_UPSTREAM}" != "true" ]]; then
  export KEYCLOAK_EXTERNAL_ADMIN_PASSWORD="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.adminPassword -r)"
  export KEYCLOAK_EXTERNAL_ADMIN_USER="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.adminUser -r)"
  export KEYCLOAK_EXTERNAL_URL="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.url -r)"
  export KEYCLOAK_INTERNAL_ADMIN_PASSWORD="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.adminPassword -r)"
  export KEYCLOAK_INTERNAL_ADMIN_USER="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.adminUser -r)"
  export KEYCLOAK_INTERNAL_URL="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.url -r)"
  SYS11_OPTIONS=-keycloak-cache-expiry=2h \
    -monitoring-environment-label=${KUBERMATIC_ENV}
else
  SYS11_OPTIONS=
fi

export KUBERMATIC_EDITION

dockercfgjson="$(mktemp)"
trap "rm -f $dockercfgjson" EXIT
cat "${INSTALLER_DIR}/kubermatic/values.yaml" | yq .kubermatic.imagePullSecretData -r | base64 --decode | jq . >"$dockercfgjson"

inClusterPrometheusRulesFile="$(mktemp)"
trap "rm -f $inClusterPrometheusRulesFile" EXIT
cat "${INSTALLER_DIR}/kubermatic/values.yaml" | yq .kubermatic.clusterNamespacePrometheus.rules >"$inClusterPrometheusRulesFile"

seedKubeconfig="$(mktemp)"
trap "rm -f $seedKubeconfig" EXIT
cp ${CONFIG_DIR}/kubeconfig $seedKubeconfig
kubectl --kubeconfig $seedKubeconfig config use-context $KUBERMATIC_CLUSTER

defaultAddons=$(cat "${INSTALLER_DIR}/kubermatic/values.yaml" | yq '.kubermatic.controller.addons.kubernetes.defaultAddons | join(",")' -r)

if [[ "${TAG_WORKER}" == "false" ]]; then
    WORKER_OPTION=
else
    WORKER_OPTION="-worker-name=$(tr -cd '[:alnum:]' <<< ${KUBERMATIC_WORKERNAME} | tr '[:upper:]' '[:lower:]')"
fi

if [[ "${DISABLE_LEADER_ELECTION}" == "true" ]]; then
    DISABLE_LE_OPTION="-enable-leader-election=false"
    LE_NAMESPACE_OPTION=
else
    DISABLE_LE_OPTION=
    if [[ "${TAG_WORKER}" == "false" ]]; then
        LE_NAMESPACE_OPTION=
    else
        LE_NAMESPACE="le-seed-$(tr -cd '[:alnum:]' <<< ${KUBERMATIC_WORKERNAME} | tr '[:upper:]' '[:lower:]')"
        kubectl create ns "$LE_NAMESPACE" || true
        LE_NAMESPACE_OPTION="-leader-election-namespace=$LE_NAMESPACE"
    fi
fi

if [[ "${DYNAMIC_DATACENTERS}" == "false" ]]; then
    DC_OPTION="-datacenters=${CONFIG_DIR}/datacenters.yaml"
elif [[ "${DYNAMIC_DATACENTERS}" == "true" ]]; then
    DC_OPTION="-dynamic-datacenters=true"
else
    DC_OPTION=
fi

# TODO extract hack/sys11-store-container.yaml / hack/sys11-cleanup-container.yaml from the installer

if [[ "${DEBUG}" == "true" ]]; then
    make KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG GOTOOLFLAGS_EXTRA="-gcflags='all=-N -l'" LDFLAGS_EXTRA="" -C ${SRC_DIR} seed-controller-manager
else
    make KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG -C ${SRC_DIR} seed-controller-manager
fi

cd ${SRC_DIR}
if [[ "${DEBUG}" == "true" ]]; then
    exec dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/seed-controller-manager -- \
      ${DC_OPTION} \
      -datacenter-name=${KUBERMATIC_CLUSTER} \
      -kubeconfig=$seedKubeconfig \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -kubernetes-addons-path=${INSTALLER_DIR}/kubermatic/cluster-addons/addons \
      -openshift-addons-path=../openshift_addons \
      -kubernetes-addons-list=$defaultAddons \
      -overwrite-registry= \
      ${SYS11_OPTIONS} \
      -feature-gates= \
      -monitoring-scrape-annotation-prefix=monitoring.metakube.de \
      -namespace=kubermatic \
      ${WORKER_OPTION} \
      -external-url=${EXTERNAL_URL} \
      -docker-pull-config-json-file="$dockercfgjson" \
      -monitoring-scrape-annotation-prefix=${KUBERMATIC_ENV} \
      -logtostderr=1 \
      -log-debug=1 \
      -backup-container=./hack/sys11-store-container.yaml \
      -cleanup-container=./hack/sys11-cleanup-container.yaml \
      -worker-count=1 \
      -kubermatic-image=${KUBERMATIC_IMAGE} \
      -dnatcontroller-image=${DNATCONTROLLER_IMAGE} \
      -backup-s3-endpoint=${S3_ENDPOINT} \
      -backup-s3-bucket=${S3_BUCKET} \
      -backup-snapshot-dir=${S3_SNAPSHOT_DIR} \
      -backup-s3-access-key=${ACCESS_KEY_ID} \
      -backup-s3-secret-access-key=${SECRET_ACCESS_KEY} \
      -etcd-launcher-image=${ETCD_LAUNCHER_IMAGE} \
      ${DISABLE_LE_OPTION} \
      ${LE_NAMESPACE_OPTION} \
      -v=8 $@

else
    exec ./_build/seed-controller-manager \
      ${DC_OPTION} \
      -datacenter-name=${KUBERMATIC_CLUSTER} \
      -kubeconfig=$seedKubeconfig \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -kubernetes-addons-path=${INSTALLER_DIR}/kubermatic/cluster-addons/addons \
      -openshift-addons-path=../openshift_addons \
      -kubernetes-addons-list=$defaultAddons \
      -overwrite-registry= \
      ${SYS11_OPTIONS} \
      -feature-gates= \
      -monitoring-scrape-annotation-prefix=monitoring.metakube.de \
      -namespace=kubermatic \
      ${WORKER_OPTION} \
      -external-url=${EXTERNAL_URL} \
      -docker-pull-config-json-file="$dockercfgjson" \
      -monitoring-scrape-annotation-prefix=${KUBERMATIC_ENV} \
      -logtostderr=1 \
      -log-debug=1 \
      -backup-container=./hack/sys11-store-container.yaml \
      -cleanup-container=./hack/sys11-cleanup-container.yaml \
      -worker-count=1 \
      -kubermatic-image=${KUBERMATIC_IMAGE} \
      -dnatcontroller-image=${DNATCONTROLLER_IMAGE} \
      -backup-s3-endpoint=${S3_ENDPOINT} \
      -backup-s3-bucket=${S3_BUCKET} \
      -backup-snapshot-dir=${S3_SNAPSHOT_DIR} \
      -backup-s3-access-key=${ACCESS_KEY_ID} \
      -backup-s3-secret-access-key=${SECRET_ACCESS_KEY} \
      -etcd-launcher-image=${ETCD_LAUNCHER_IMAGE} \
      ${DISABLE_LE_OPTION} \
      ${LE_NAMESPACE_OPTION} \
      -v=6 $@

      # TODO
      #-in-cluster-prometheus-rules-file="$inClusterPrometheusRulesFile" \
      #-seed-admissionwebhook-cert-file=/opt/seed-webhook-serving-cert/serverCert.pem
      #-seed-admissionwebhook-key-file=/opt/seed-webhook-serving-cert/serverKey.pem
      #-oidc-ca-file=../../secrets/seed-clusters/dev.kubermatic.io/caBundle.pem \
      #-oidc-issuer-url=$(vault kv get -field=oidc-issuer-url dev/seed-clusters/dev.kubermatic.io) \
      #-oidc-issuer-client-id=$(vault kv get -field=oidc-issuer-client-id dev/seed-clusters/dev.kubermatic.io) \
      #-oidc-issuer-client-secret=$(vault kv get -field=oidc-issuer-client-secret dev/seed-clusters/dev.kubermatic.io) \
fi
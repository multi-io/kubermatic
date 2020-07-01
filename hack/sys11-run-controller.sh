#!/bin/bash

set -o errexit
set -o pipefail
set -x

: "${SRC_DIR:=$(go env GOPATH)/src/github.com/kubermatic/kubermatic/api}"
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
: "${KUBERMATICCOMMIT:="$([[ -n "$(git tag --points-at)" ]] && git tag --points-at || git log -1 --format=%H)"}"
: "${GITTAG:=$(git describe --tags --always)}"
: "${DYNAMIC_DATACENTERS:="false"}"   # true | false | absent  -- absent meaning pass neither -datacenters= nor -dynamic-datacenters= (2.15+)
: "${IS_KUBERMATIC_UPSTREAM:="false"}"
: "${KUBERMATIC_IMAGE:="docker.io/syseleven/kubermatic"}"
: "${DNATCONTROLLER_IMAGE:="docker.io/syseleven/kubeletdnat-controller"}"
: "${ETCD_LAUNCHER_IMAGE_BASE:="docker.io/syseleven/etcd-launcher-"}"
: "${KUBERMATIC_EDITION:=ee}"

# $KUBERMATICCOMMIT and $GITTAG must refer to git tag names for which we've built and uploaded kubermatic images
# (because those tags will set as image tag for user cluster apiserver pod sidecar containers, e.g. the
# docker.io/syseleven/kubeletdnat-controller image)
# If they don't, e.g. because you're running with locally committed and not yet pushed changes, you must
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
    DISABLE_LE_OPTION="-disable-leader-election"
else
    DISABLE_LE_OPTION=
fi

if [[ "${DYNAMIC_DATACENTERS}" == "false" ]]; then
    DC_OPTION="-datacenters=${CONFIG_DIR}/datacenters.yaml"
elif [[ "${DYNAMIC_DATACENTERS}" == "true" ]]; then
    DC_OPTION="-dynamic-datacenters=true"
else
    DC_OPTION=
fi

# TODO extract hack/sys11-store-container.yaml / hack/sys11-cleanup-container.yaml from the installer

while true; do
    if [[ "${DEBUG}" == "true" ]]; then
        make KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG GOTOOLFLAGS_EXTRA="-gcflags='all=-N -l'" LDFLAGS_EXTRA="" -C ${SRC_DIR} seed-controller-manager
    else
        make KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG -C ${SRC_DIR} seed-controller-manager
    fi

    cd ${SRC_DIR}
    if [[ "${DEBUG}" == "true" ]]; then
        dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/seed-controller-manager -- \
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
          -etcd-launcher-image-base=${ETCD_LAUNCHER_IMAGE_BASE} \
          ${DISABLE_LE_OPTION} \
          -v=8 $@ &

        PID=$!
    else
        ./_build/seed-controller-manager \
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
          -etcd-launcher-image-base=${ETCD_LAUNCHER_IMAGE_BASE} \
          ${DISABLE_LE_OPTION} \
          -v=6 $@ &

          # TODO
          #-in-cluster-prometheus-rules-file="$inClusterPrometheusRulesFile" \
          #-seed-admissionwebhook-cert-file=/opt/seed-webhook-serving-cert/serverCert.pem
          #-seed-admissionwebhook-key-file=/opt/seed-webhook-serving-cert/serverKey.pem
          #-oidc-ca-file=../../secrets/seed-clusters/dev.kubermatic.io/caBundle.pem \
          #-oidc-issuer-url=$(vault kv get -field=oidc-issuer-url dev/seed-clusters/dev.kubermatic.io) \
          #-oidc-issuer-client-id=$(vault kv get -field=oidc-issuer-client-id dev/seed-clusters/dev.kubermatic.io) \
          #-oidc-issuer-client-secret=$(vault kv get -field=oidc-issuer-client-secret dev/seed-clusters/dev.kubermatic.io) \

        PID=$!
    fi



    if [[ -x "$(command -v inotifywait)" ]]; then
        inotifywait -r -e modify ${SRC_DIR}
    elif [[ -x "$(command -v fswatch)" ]]; then
        fswatch --one-event ${SRC_DIR}
    else
        echo "Can not watch changes because neither fswatch (MAC) nor inotifywait found"
        kill ${PID}
        exit 1
    fi


    echo "Change in kubermatic detected, recompiling and restarting"

    kill ${PID} || true
done

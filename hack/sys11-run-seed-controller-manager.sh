#!/bin/bash

# set -o errexit
# set -o pipefail
# set -x

cd "$(dirname $0)"

: "${SRC_DIR:=$(cd ..; pwd)}"
cd ${SRC_DIR}
: "${SYS11_HACK_DIR:="${SRC_DIR}/hack"}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-${USERNAME}}}"
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"

ENV_CLUSTER_DIR="${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/clusters/${KUBERMATIC_CLUSTER}"

: "${ETCD_LAUNCHER_IMAGE:=$(cat "${ENV_CLUSTER_DIR}/kubermatic/variables/kubermatic.yaml" | yq .etcd_launcher_image -r)}"
: "${CONFIG_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/kubermatic}"
if [[ -z "$SKIP_INSTALLER" ]]; then
KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} make -C ${INSTALLER_DIR}/kubermatic values.yaml
fi
: "${EXTERNAL_URL:=$(cat "${ENV_CLUSTER_DIR}/kubermatic/variables/base.yaml" | yq .public_base_domain -r)}"
: "${DEBUG:="false"}"
: "${KUBERMATICCOMMIT:="$(git rev-parse HEAD)"}"
: "${GITTAG:=$(git describe --tags --always)}"
: "${KUBERMATIC_EDITION:=ee}"
: "${IS_KUBERMATIC_UPSTREAM:="false"}"

export KUBERMATIC_EDITION

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
  SYS11_OPTIONS="-keycloak-cache-expiry=2h -monitoring-environment-label=${KUBERMATIC_ENV}"
else
  SYS11_OPTIONS=
fi

tmpdir="$(mktemp -d)"
trap "rm -rf $tmpdir" EXIT

cat "${INSTALLER_DIR}/kubermatic/values.yaml" | yq .kubermatic.imagePullSecretData -r | base64 --decode | jq . >"${tmpdir}/dockercfgjson"

cat "${INSTALLER_DIR}/kubermatic/additional-projects/kubermaticconfiguration/kubermaticconfiguration.yaml" | yq '.spec.userCluster.monitoring.customRules | select(.)' -r >"${tmpdir}/inClusterPrometheusRulesFile"

defaultAddons=$(cat "${ENV_CLUSTER_DIR}/kubermatic/variables/kubermatic.yaml" | yq '.kubermatic_default_addons | join(",")' -r)

## Would need format conversion as in CreateVersionsYAML() / CreateUpdatesYAML() in pkg/controller/operator/common/defaults.go
#cat "${ENV_CLUSTER_DIR}/kubermatic/variables/kubermatic.yaml" | yq '{versions: .versions_and_updates.kubernetes.versions}' --yaml-output >"${tmpdir}/versions"
#cat "${ENV_CLUSTER_DIR}/kubermatic/variables/kubermatic.yaml" | yq '{updates: .versions_and_updates.kubernetes.updates}' --yaml-output >"${tmpdir}/updates"
## => read from seed cluster directly
kubectl -n kubermatic get secret extra-files -o json | jq '.data."versions.yaml"' -r | base64 --decode >"${tmpdir}/versions"
kubectl -n kubermatic get secret extra-files -o json | jq '.data."updates.yaml"' -r | base64 --decode >"${tmpdir}/updates"

kubectl -n kubermatic get secret extra-files -o json | jq '.data."provider-incompatibilities.yaml"' -r | base64 --decode >"${tmpdir}/provider-incompatibilities"

if [[ "${TAG_WORKER}" == "false" ]]; then
    WORKER_OPTION=
else
    WORKER_OPTION="-worker-name=$(tr -cd '[:alnum:]' <<< ${KUBERMATIC_WORKERNAME} | tr '[:upper:]' '[:lower:]')"
fi

if [[ "${DISABLE_LEADER_ELECTION}" == "false" ]]; then
    DISABLE_LE_OPTION=
    if [[ "${TAG_WORKER}" == "false" ]]; then
        LE_NAMESPACE_OPTION=
    else
        LE_NAMESPACE="le-seed-$(tr -cd '[:alnum:]' <<< ${KUBERMATIC_WORKERNAME} | tr '[:upper:]' '[:lower:]')"
        kubectl create ns "$LE_NAMESPACE" || true
        LE_NAMESPACE_OPTION="-leader-election-namespace=$LE_NAMESPACE"
    fi
else
    DISABLE_LE_OPTION="-enable-leader-election=false"
    LE_NAMESPACE_OPTION=
fi

# TODO extract hack/sys11-store-container.yaml / hack/sys11-cleanup-container.yaml from the installer

if [[ "${DEBUG}" == "true" ]]; then
    make KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG GOTOOLFLAGS_EXTRA="-gcflags='all=-N -l'" LDFLAGS_EXTRA="" -C ${SRC_DIR} seed-controller-manager
    RUNCMD="dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/seed-controller-manager --"
else
    make KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG seed-controller-manager
    RUNCMD=./_build/seed-controller-manager
fi

# needed by addon controller
export KUBERMATIC_KUBECTL_OVERRIDE="$(which kubectl)"

$RUNCMD \
  -ca-bundle=${INSTALLER_DIR}/kubermatic/kubermatic-code/charts/kubermatic-operator/static/ca-bundle.pem \
  -controller-manager-default-replicas=1 \
  -dynamic-datacenters=true \
  -datacenter-name=${KUBERMATIC_CLUSTER} \
  -enable-etcd-backups-restores \
  -etcd-disk-size=$(cat "${ENV_CLUSTER_DIR}/kubermatic/variables/kubermatic.yaml" | yq '.customer_etcd_disk_size' -r) \
  -versions="${tmpdir}/versions" \
  -updates="${tmpdir}/updates" \
  -provider-incompatibilities="${tmpdir}/provider-incompatibilities" \
  -kubernetes-addons-path=${INSTALLER_DIR}/kubermatic/cluster-addons/addons \
  -kubernetes-addons-list=$defaultAddons \
  -overwrite-registry= \
  -nodeport-range=30000-32767 \
  -docker-pull-config-json-file="${tmpdir}/dockercfgjson" \
  -feature-gates=etcdLauncher=true \
  ${SYS11_OPTIONS} \
  -monitoring-scrape-annotation-prefix=monitoring.metakube.de \
  -monitoring-environment-label="$(cat "${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/variables/base.yaml" | yq '.environment' -r)" \
  -namespace=kubermatic \
  ${WORKER_OPTION} \
  -external-url=${EXTERNAL_URL} \
  -in-cluster-prometheus-disable-default-rules=false \
  -in-cluster-prometheus-disable-default-scraping-configs=false \
  -in-cluster-prometheus-rules-file="${tmpdir}/inClusterPrometheusRulesFile" \
  -logtostderr=1 \
  -log-debug=1 \
  -scheduler-default-replicas=1 \
  -backup-container=./hack/sys11-store-container.yaml \
  -cleanup-container=./hack/sys11-cleanup-container.yaml \
  -backup-delete-container=./hack/sys11-delete-container.yaml \
  -worker-count=1 \
  -kubermatic-image=docker.io/syseleven/kubermatic \
  -dnatcontroller-image=syseleven/kubeletdnat-controller \
  -etcd-launcher-image="${ETCD_LAUNCHER_IMAGE}" \
  ${DISABLE_LE_OPTION} \
  ${LE_NAMESPACE_OPTION} \
  -v=8 $@

  # TODO
  #-oidc-ca-file=../secrets/seed-clusters/dev.kubermatic.io/caBundle.pem \
  #-oidc-issuer-url=$(vault kv get -field=oidc-issuer-url dev/seed-clusters/dev.kubermatic.io) \
  #-oidc-issuer-client-id=$(vault kv get -field=oidc-issuer-client-id dev/seed-clusters/dev.kubermatic.io) \
  #-oidc-issuer-client-secret=$(vault kv get -field=oidc-issuer-client-secret dev/seed-clusters/dev.kubermatic.io) \

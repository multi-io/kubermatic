#!/bin/bash

# set -o errexit
# set -o pipefail
# set -x

: "${SRC_DIR:=$(go env GOPATH)/src/k8c.io/kubermatic}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-$(uname -n)}}"
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

# $KUBERMATICCOMMIT and $GITTAG must refer to git tag names for which we've built and uploaded kubermatic images
# (because those tags will set as image tag for user cluster apiserver pod sidecar containers, e.g. the
# docker.io/syseleven/kubeletdnat-controller image)
# If they don't, e.g. because you're running with locally committed and not yet pushed changes, you must
# override those variables, e.g. KUBERMATICCOMMIT=v2.13.1-sys11-12 GITTAG=v2.13.1-sys11-12 ... ./sys11-run-controller.sh

export KEYCLOAK_EXTERNAL_ADMIN_PASSWORD="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.adminPassword -r)"
export KEYCLOAK_EXTERNAL_ADMIN_USER="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.adminUser -r)"
export KEYCLOAK_EXTERNAL_URL="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.url -r)"
export KEYCLOAK_INTERNAL_ADMIN_PASSWORD="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.adminPassword -r)"
export KEYCLOAK_INTERNAL_ADMIN_USER="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.adminUser -r)"
export KEYCLOAK_INTERNAL_URL="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.url -r)"

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

# TODO extract hack/sys11-store-container.yaml / hack/sys11-cleanup-container.yaml from the installer

if [[ "${DEBUG}" == "true" ]]; then
    make KUBERMATIC_EDITION="ee" KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG GOBUILDFLAGS="-gcflags='all=-N -l'" seed-controller-manager
else
    make KUBERMATIC_EDITION="ee" KUBERMATICCOMMIT=$KUBERMATICCOMMIT GITTAG=$GITTAG seed-controller-manager
fi

cd ${SRC_DIR}
if [[ "${DEBUG}" == "true" ]]; then
    dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/seed-controller-manager -- \
      -dynamic-datacenters=true \
      -datacenter-name=${KUBERMATIC_CLUSTER} \
      -kubeconfig=$seedKubeconfig \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -kubernetes-addons-path=${INSTALLER_DIR}/kubermatic/cluster-addons/addons \
      -openshift-addons-path=openshift_addons \
      -kubernetes-addons-list=$defaultAddons \
      -overwrite-registry= \
      -nodeport-range=30000-32767 \
      -docker-pull-config-json-file="$dockercfgjson" \
      -feature-gates=etcdLauncher=true \
      -keycloak-cache-expiry=2h \
      -monitoring-environment-label=${KUBERMATIC_ENV} \
      -monitoring-scrape-annotation-prefix=monitoring.metakube.de \
      -namespace=kubermatic \
      -worker-name=${KUBERMATIC_WORKERNAME} \
      -external-url=${EXTERNAL_URL} \
      -logtostderr=1 \
      -log-debug=1 \
      -backup-container=./hack/sys11-store-container.yaml \
      -cleanup-container=./hack/sys11-cleanup-container.yaml \
      -backup-delete-container=./hack/sys11-delete-container.yaml \
      -default-component-settings=./hack/sys11-default-component-settings.yaml \
      -worker-count=1 \
      -kubermatic-image=docker.io/syseleven/kubermatic \
      -dnatcontroller-image=syseleven/kubeletdnat-controller \
      -enable-leader-election=false \
      ${DISABLE_LE_OPTION} \
      -v=8 $@

    PID=$!
else
    ./_build/seed-controller-manager \
      -dynamic-datacenters=true \
      -datacenter-name=${KUBERMATIC_CLUSTER} \
      -kubeconfig=$seedKubeconfig \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -kubernetes-addons-path=${INSTALLER_DIR}/kubermatic/cluster-addons/addons \
      -openshift-addons-path=openshift_addons \
      -kubernetes-addons-list=$defaultAddons \
      -overwrite-registry= \
      -nodeport-range=30000-32767 \
      -docker-pull-config-json-file="$dockercfgjson" \
      -feature-gates=etcdLauncher=true \
      -keycloak-cache-expiry=2h \
      -monitoring-environment-label=${KUBERMATIC_ENV} \
      -monitoring-scrape-annotation-prefix=monitoring.metakube.de \
      -namespace=kubermatic \
      -worker-name=${KUBERMATIC_WORKERNAME} \
      -external-url=${EXTERNAL_URL} \
      -logtostderr=1 \
      -log-debug=1 \
      -backup-container=./hack/sys11-store-container.yaml \
      -cleanup-container=./hack/sys11-cleanup-container.yaml \
      -backup-delete-container=./hack/sys11-delete-container.yaml \
      -default-component-settings=./hack/sys11-default-component-settings.yaml \
      -worker-count=1 \
      -kubermatic-image=docker.io/syseleven/kubermatic \
      -dnatcontroller-image=syseleven/kubeletdnat-controller \
      -enable-leader-election=false \
      ${DISABLE_LE_OPTION} \
      -v=4 $@

      # TODO
      #-in-cluster-prometheus-rules-file="$inClusterPrometheusRulesFile" \
      #-seed-admissionwebhook-cert-file=/opt/seed-webhook-serving-cert/serverCert.pem
      #-seed-admissionwebhook-key-file=/opt/seed-webhook-serving-cert/serverKey.pem
      #-oidc-ca-file=../secrets/seed-clusters/dev.kubermatic.io/caBundle.pem \
      #-oidc-issuer-url=$(vault kv get -field=oidc-issuer-url dev/seed-clusters/dev.kubermatic.io) \
      #-oidc-issuer-client-id=$(vault kv get -field=oidc-issuer-client-id dev/seed-clusters/dev.kubermatic.io) \
      #-oidc-issuer-client-secret=$(vault kv get -field=oidc-issuer-client-secret dev/seed-clusters/dev.kubermatic.io) \
fi

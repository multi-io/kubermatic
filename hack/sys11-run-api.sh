#!/bin/bash

set -o errexit
set -o pipefail
set -x

: "${SRC_DIR:=$(go env GOPATH)/src/github.com/kubermatic/kubermatic}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-${USERNAME}}}"
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"
: "${RESOURCES_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/clusters/${KUBERMATIC_CLUSTER}/kubermatic/versions}"
: "${CONFIG_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/kubermatic}"
if [[ -z "$SKIP_INSTALLER" ]]; then
KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} make -C ${INSTALLER_DIR}/kubermatic values.yaml
fi
: "${DEBUG:="false"}"
: "${TAG_WORKER:="true"}"
: "${DYNAMIC_DATACENTERS:="false"}"   # true | false | absent  -- absent meaning pass neither -datacenters= nor -dynamic-datacenters= (2.15+)
: "${KUBERMATIC_EDITION:=ee}"

export KUBERMATIC_EDITION

SERVICE_ACCOUNT_SIGNING_KEY="$(KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} ${INSTALLER_DIR}/bin/run-vault kv get -field=serviceAccountKey secret/metakube-${KUBERMATIC_ENV}/clusters/dbl1/kubermatic/auth)"

export KEYCLOAK_INTERNAL_URL="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.url -r)"
export KEYCLOAK_INTERNAL_ADMIN_USER="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.adminUser -r)"
export KEYCLOAK_INTERNAL_ADMIN_PASSWORD="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.internal.adminPassword -r)"
export KEYCLOAK_EXTERNAL_URL="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.url -r)"
export KEYCLOAK_EXTERNAL_ADMIN_USER="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.adminUser -r)"
export KEYCLOAK_EXTERNAL_ADMIN_PASSWORD="$(cat ${INSTALLER_DIR}/kubermatic/values.yaml | yq .keycloak.external.adminPassword -r)"

if [[ "${TAG_WORKER}" == "false" ]]; then
    WORKER_OPTION=
else
    WORKER_OPTION="-worker-name=$(tr -cd '[:alnum:]' <<< ${KUBERMATIC_WORKERNAME} | tr '[:upper:]' '[:lower:]')"
fi

if [[ "${DYNAMIC_DATACENTERS}" == "false" ]]; then
    DC_OPTION="-datacenters=${CONFIG_DIR}/datacenters.yaml"
elif [[ "${DYNAMIC_DATACENTERS}" == "true" ]]; then
    DC_OPTION="-dynamic-datacenters=true"
else
    DC_OPTION=
fi

if [[ "${DEBUG}" == "true" ]]; then
    GOTOOLFLAGS="-v -gcflags='all=-N -l'" make -C ${SRC_DIR} kubermatic-api
else
    make -C ${SRC_DIR} kubermatic-api
fi

# Please make sure to set -feature-gates=PrometheusEndpoint=true if you want to use that endpoint.

# Please make sure to set -feature-gates=OIDCKubeCfgEndpoint=true if you want to use that endpoint.
# Note that you would have to pass a few additional flags as well.

cd ${SRC_DIR}
if [[ "${DEBUG}" == "true" ]]; then
    dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/kubermatic-api -- \
      -kubeconfig=${CONFIG_DIR}/kubeconfig \
      ${DC_OPTION} \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -master-resources=${RESOURCES_DIR} \
      ${WORKER_OPTION} \
      -internal-address=127.0.0.1:18085 \
      -prometheus-url=http://localhost:9090 \
      -address=127.0.0.1:8080 \
      -oidc-url=https://keystone-oidc.app.syseleven-dbl1-1.dev.metakube.de \
      -oidc-authenticator-client-id=metakube-dashboard \
      -oidc-skip-tls-verify=false \
      -service-account-signing-key="${SERVICE_ACCOUNT_SIGNING_KEY}" \
      -accessible-addons dashboard,metakube-ark,metakube-autoscaler,metakube-backups,metakube-helm,metakube-ingress,metakube-monitoring,metakube-weave-scope,metakube-webterminal,metakube-vault \
      -logtostderr \
      -v=8 $@

    PID=$!
else
    ./_build/kubermatic-api \
      -kubeconfig=${CONFIG_DIR}/kubeconfig \
      ${DC_OPTION} \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -master-resources=${RESOURCES_DIR} \
      ${WORKER_OPTION} \
      -internal-address=127.0.0.1:18085 \
      -prometheus-url=http://localhost:9090 \
      -address=127.0.0.1:8080 \
      -oidc-url=https://keystone-oidc.app.syseleven-dbl1-1.dev.metakube.de \
      -oidc-authenticator-client-id=metakube-dashboard \
      -oidc-skip-tls-verify=false \
      -service-account-signing-key="${SERVICE_ACCOUNT_SIGNING_KEY}" \
      -accessible-addons dashboard,metakube-ark,metakube-autoscaler,metakube-backups,metakube-helm,metakube-ingress,metakube-monitoring,metakube-weave-scope,metakube-webterminal,metakube-vault \
      -logtostderr \
      -v=8 $@

    PID=$!
fi
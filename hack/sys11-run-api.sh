#!/bin/bash

set -o errexit
set -o pipefail
set -x

: "${SRC_DIR:=$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic}"
: "${KUBERMATIC_WORKERNAME:=${KUBERMATIC_WORKERNAME:-$(uname -n)}}"
: "${INSTALLER_DIR:="$(go env GOPATH)/src/gitlab.syseleven.de/kubernetes/kubermatic-installer"}"
: "${KUBERMATIC_ENV:=dev}"
: "${KUBERMATIC_CLUSTER:=dbl1}"
: "${CONFIG_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/kubermatic}"
: "${CLUSTER_CONFIG_DIR:=${INSTALLER_DIR}/environments/${KUBERMATIC_ENV}/clusters/${KUBERMATIC_CLUSTER}}"
export KUBERMATIC_EDITION="ce"
if [[ -z "$SKIP_INSTALLER" ]]; then
KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} make -C ${INSTALLER_DIR}/kubermatic values.yaml
fi
: "${DEBUG:="false"}"
: "${TAG_WORKER:="true"}"
SERVICE_ACCOUNT_SIGNING_KEY="$(KUBERMATIC_ENV=${KUBERMATIC_ENV} KUBERMATIC_CLUSTER=${KUBERMATIC_CLUSTER} ${INSTALLER_DIR}/bin/run-vault kv get -field=serviceAccountKey secret/metakube-${KUBERMATIC_ENV}/clusters/dbl1/kubermatic/auth)"

# TODO generate versions and updates files from the environment's kubermaticconfiguration
RESOURCES_DIR="${SRC_DIR}/hack/sys11-resources-dir"

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

if [[ "${DEBUG}" == "true" ]]; then
    GOTOOLFLAGS="-v -gcflags='all=-N -l'" make -C ${SRC_DIR} KUBERMATIC_EDITION=${KUBERMATIC_EDITION} kubermatic-api
else
    make -C ${SRC_DIR} KUBERMATIC_EDITION=${KUBERMATIC_EDITION} kubermatic-api
fi

: "${OIDC_URL:=https://keystone-oidc.$(${INSTALLER_DIR}/bin/getvar kubermatic base.app_base_domain)}"

# Please make sure to set -feature-gates=PrometheusEndpoint=true if you want to use that endpoint.

# Please make sure to set -feature-gates=OIDCKubeCfgEndpoint=true if you want to use that endpoint.
# Note that you would have to pass a few additional flags as well.

cd ${SRC_DIR}
if [[ "${DEBUG}" == "true" ]]; then
    dlv --listen=:2345 --headless=true --api-version=2 --accept-multiclient exec ./_build/kubermatic-api -- \
      -kubeconfig=${CLUSTER_CONFIG_DIR}/kubeconfig/useradmin.conf \
      -dynamic-presets=true \
      -swagger=cmd/kubermatic-api/swagger.json \
      -ca-bundle=charts/kubermatic-operator/static/ca-bundle.pem \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -provider-incompatibilities=${RESOURCES_DIR}/provider-incompatibilities.yaml \
      -master-resources=${RESOURCES_DIR} \
      ${WORKER_OPTION} \
      -internal-address=127.0.0.1:18085 \
      -prometheus-url=http://localhost:9090 \
      -address=127.0.0.1:8080 \
      -oidc-url=$OIDC_URL \
      -oidc-authenticator-client-id=metakube-dashboard \
      -oidc-skip-tls-verify=false \
      -service-account-signing-key="${SERVICE_ACCOUNT_SIGNING_KEY}" \
      -accessible-addons dashboard,metakube-ark,cluster-autoscaler,metakube-backups,metakube-helm,metakube-ingress,metakube-monitoring,metakube-weave-scope,metakube-webterminal,metakube-vault \
      -logtostderr \
      -v=8 $@
else
    ./_build/kubermatic-api \
      -kubeconfig=${CLUSTER_CONFIG_DIR}/kubeconfig/useradmin.conf \
      -dynamic-presets=true \
      -swagger=cmd/kubermatic-api/swagger.json \
      -ca-bundle=charts/kubermatic-operator/static/ca-bundle.pem \
      -versions=${RESOURCES_DIR}/versions.yaml \
      -updates=${RESOURCES_DIR}/updates.yaml \
      -provider-incompatibilities=${RESOURCES_DIR}/provider-incompatibilities.yaml \
      -master-resources=${RESOURCES_DIR} \
      ${WORKER_OPTION} \
      -internal-address=127.0.0.1:18085 \
      -prometheus-url=http://localhost:9090 \
      -address=127.0.0.1:8080 \
      -oidc-url=$OIDC_URL \
      -oidc-authenticator-client-id=metakube-dashboard \
      -oidc-skip-tls-verify=false \
      -service-account-signing-key="${SERVICE_ACCOUNT_SIGNING_KEY}" \
      -accessible-addons dashboard,metakube-ark,cluster-autoscaler,metakube-backups,metakube-helm,metakube-ingress,metakube-monitoring,metakube-weave-scope,metakube-webterminal,metakube-vault \
      -logtostderr \
      -v=8 $@
fi

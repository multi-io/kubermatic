module k8c.io/kubermatic/v2

go 1.14

require (
	code.cloudfoundry.org/go-pubsub v0.0.0-20180503211407-becd51dc37cb
	github.com/Azure/azure-sdk-for-go v49.0.0+incompatible
	github.com/Azure/go-autorest/autorest v0.11.13
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.5
	github.com/Azure/go-autorest/autorest/to v0.4.0
	github.com/Masterminds/semver/v3 v3.1.1
	github.com/Masterminds/sprig v2.22.0+incompatible
	github.com/Masterminds/sprig/v3 v3.1.0 // indirect
	github.com/aliyun/alibaba-cloud-sdk-go v1.61.751
	github.com/apoydence/onpar v0.0.0-20200406201722-06f95a1c68e8 // indirect
	github.com/apparentlymart/go-cidr v1.1.0
	github.com/aws/aws-sdk-go v1.36.2
	github.com/coreos/go-oidc v2.2.1+incompatible
	github.com/coreos/locksmith v0.6.2
	github.com/cristim/ec2-instances-info v0.0.0-20190708120723-b53a9860c46d
	github.com/digitalocean/godo v1.54.0
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/spdystream v0.0.0-20181023171402-6480d4af844c // indirect
	github.com/envoyproxy/go-control-plane v0.9.4
	github.com/evanphx/json-patch v4.9.0+incompatible
	github.com/ghodss/yaml v1.0.0
	github.com/go-ini/ini v1.55.0 // indirect
	github.com/go-kit/kit v0.10.0
	github.com/go-logr/zapr v0.1.1
	github.com/go-openapi/errors v0.19.6
	github.com/go-openapi/runtime v0.19.20
	github.com/go-openapi/strfmt v0.19.5
	github.com/go-openapi/swag v0.19.9
	github.com/go-openapi/validate v0.19.10
	github.com/go-swagger/go-swagger v0.25.0
	github.com/go-test/deep v1.0.7
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.3
	github.com/gophercloud/gophercloud v0.14.0
	github.com/gorilla/handlers v1.4.2
	github.com/gorilla/mux v1.8.0
	github.com/gorilla/securecookie v1.1.1
	github.com/gorilla/websocket v1.4.2
	github.com/heptiolabs/healthcheck v0.0.0-20180807145615-6ff867650f40
	github.com/hetznercloud/hcloud-go v1.23.1
	github.com/huandu/xstrings v1.3.2 // indirect
	github.com/iancoleman/strcase v0.1.2
	github.com/imdario/mergo v0.3.11 // indirect
	github.com/jetstack/cert-manager v1.1.0
	github.com/kubermatic/machine-controller v1.23.1
	github.com/minio/minio-go v6.0.14+incompatible
	github.com/nelsam/hel v0.0.0-20200611165952-2d829bae0c66 // indirect
	github.com/oklog/run v1.1.0
	github.com/onsi/ginkgo v1.14.0
	github.com/packethost/packngo v0.1.1-0.20190410075950-a02c426e4888
	github.com/pkg/errors v0.9.1
	github.com/pmezard/go-difflib v1.0.0
	github.com/poy/onpar v0.0.0-20200406201722-06f95a1c68e8 // indirect
	github.com/prometheus/client_golang v1.8.0
	github.com/robfig/cron v1.2.0
	github.com/sirupsen/logrus v1.6.0
	github.com/spf13/cobra v1.0.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.6.1
	github.com/urfave/cli v1.22.4
	github.com/vmware/govmomi v0.23.1
	go.etcd.io/etcd/v3 v3.3.0-rc.0.0.20200728214110-6c81b20ec8de
	go.uber.org/zap v1.16.0
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392
	golang.org/x/oauth2 v0.0.0-20201109201403-9fd604954f58
	golang.org/x/tools v0.0.0-20201202200335-bef1c476418a
	google.golang.org/api v0.36.0
	google.golang.org/grpc v1.33.2
	gopkg.in/fsnotify.v1 v1.4.7
	gopkg.in/square/go-jose.v2 v2.5.1
	gopkg.in/yaml.v2 v2.3.0
	gopkg.in/yaml.v3 v3.0.0-20200615113413-eeeca48fe776
	k8s.io/api v0.19.4
	k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery v0.19.4
	k8s.io/autoscaler v0.0.0-20190218140445-7f77136aeea4 // git digest for VPA v0.4.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/code-generator v0.19.4
	k8s.io/klog v1.0.0
	k8s.io/klog/v2 v2.3.0 // indirect
	k8s.io/kube-aggregator v0.19.0
	k8s.io/metrics v0.19.0
	k8s.io/test-infra v0.0.0-20200220102703-18fae0a00a2c
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
	sigs.k8s.io/controller-runtime v0.7.0
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/go-logr/logr => github.com/go-logr/logr v0.2.1-0.20200730175230-ee2de8da5be6
	github.com/go-logr/zapr => github.com/go-logr/zapr v0.2.0
	// pin to prevent panics in DeepEqual() when comparing envoy objects
	github.com/golang/protobuf => github.com/golang/protobuf v1.3.2
	// prevent 2.0.0 dependency because it's a broken release
	gomodules.xyz/jsonpatch/v2 => gomodules.xyz/jsonpatch/v2 v2.1.0
	// pin to prevent panics in DeepEqual() when comparing envoy objects
	google.golang.org/genproto => google.golang.org/genproto v0.0.0-20190219182410-082222b4a5c5
	// etcd.v3 needs an old version for the google.golang.org/grpc/naming package, which got removed in grpc 1.30+
	google.golang.org/grpc => google.golang.org/grpc v1.29.1
	k8s.io/api => k8s.io/api v0.19.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.19.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.19.4
	k8s.io/client-go => k8s.io/client-go v0.19.4
	k8s.io/code-generator => k8s.io/code-generator v0.19.4
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.19.4
	k8s.io/kubelet => k8s.io/kubelet v0.19.4
	k8s.io/metrics => k8s.io/metrics v0.19.4
	// stick to an older version until we have time to update
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.6.4
)

replace github.com/kubermatic/machine-controller => github.com/syseleven/machine-controller v1.24.4-sys11-1

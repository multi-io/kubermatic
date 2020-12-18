package clusterproxy

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/Masterminds/sprig"

	"github.com/kubermatic/kubermatic/api/pkg/resources"
	"github.com/kubermatic/kubermatic/api/pkg/resources/reconciling"

	corev1 "k8s.io/api/core/v1"
)

type nginxTplModel struct {
	NginxPort           int
	LinkerdNginxPort    int
	DNSConfigResolverIP string
	ServerName          string
}

// ConfigMapCreator returns a ConfigMapCreator containing the nginx config for the supplied data
func ConfigMapCreator(data *resources.TemplateData) reconciling.NamedConfigMapCreatorGetter {
	return func() (string, reconciling.ConfigMapCreator) {
		return resources.ClusterProxyConfigConfigMapName, func(cm *corev1.ConfigMap) (*corev1.ConfigMap, error) {
			if cm.Data == nil {
				cm.Data = map[string]string{}
			}

			dnsConfigResolverIP, err := data.ClusterIPByServiceName(resources.DNSResolverServiceName)
			if err != nil {
				return nil, fmt.Errorf("can not get dns resolver IP: %v", err)
			}

			seedDC := data.Seed().Name

			if seedDC == "dbl1" {
				seedDC = "syseleven-dbl1-1"
			} else if seedDC == "bki1" {
				seedDC = "syseleven-dbl1-1"
			}

			model := &nginxTplModel{
				NginxPort:           nginxPort,
				LinkerdNginxPort:    linkerdNginxPort,
				DNSConfigResolverIP: dnsConfigResolverIP,
				ServerName:          fmt.Sprintf("clusterproxy-%s.app.%s.%s", data.Cluster().Name, seedDC, data.ExternalURL),
			}

			configBuffer := bytes.Buffer{}
			configTpl, err := template.New("base").Funcs(sprig.TxtFuncMap()).Parse(nginxConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to parse nginx config template: %v", err)
			}
			if err := configTpl.Execute(&configBuffer, model); err != nil {
				return nil, fmt.Errorf("failed to render nginx config template: %v", err)
			}

			cm.Labels = resources.BaseAppLabels(name, nil)
			cm.Data["server.conf"] = configBuffer.String()

			return cm, nil
		}
	}
}

const nginxConfig = `map $http_upgrade $connection_upgrade {
	  default upgrade;
	  '' close;
  }

  server {
      listen {{ .LinkerdNginxPort }};
	  server_name {{ .ServerName }};
	  resolver {{ .DNSConfigResolverIP }} valid=30s;

      location /healthz {
          access_log off;
          return 200 "healthy\n";
      }

	  location / {
		  set $upstream "linkerd-web.linkerd.svc.cluster.local:8084";
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
          proxy_set_header Origin "";
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
  }

  server {
	  listen {{ .NginxPort }};
	  server_name {{ .ServerName }};
	  resolver {{ .DNSConfigResolverIP }} valid=30s;

      location /healthz {
          access_log off;
          return 200 "healthy\n";
      }
	  location = /prometheus {
		  return 302 https://$server_name/prometheus/;
	  }
	  location /prometheus/ {
		  set $upstream "syseleven-monitoring-prome-prometheus.syseleven-monitoring.svc.cluster.local:9090";
		  rewrite ^/prometheus(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
	  }
	  location = /grafana {
		  return 302 https://$server_name/grafana/;
	  }
	  location /grafana/ {
		  set $upstream "syseleven-monitoring-grafana.syseleven-monitoring.svc.cluster.local:80";
		  rewrite ^/grafana(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
	  }
	  location = /alertmanager {
		  return 302 https://$server_name/alertmanager/;
	  }
	  location /alertmanager/ {
		  set $upstream "syseleven-monitoring-prome-alertmanager.syseleven-monitoring.svc.cluster.local:9093";
		  rewrite ^/alertmanager(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
	  }
	  location = /webterminal {
		  return 302 https://$server_name/webterminal/;
	  }
	  location /webterminal/ {
		  set $upstream "webterminal.webterminal.svc.cluster.local:9000";
		  rewrite ^/webterminal(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
	  location = /weave-scope {
		  return 302 https://$server_name/weave-scope/;
	  }
	  location /weave-scope/ {
		  set $upstream "syseleven-weave-scope-weave-scope.syseleven-weave-scope.svc.cluster.local:80";
		  rewrite ^/weave-scope(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
	  location = /kubernetes-dashboard {
		  return 302 https://$server_name/kubernetes-dashboard/;
	  }
	  location /kubernetes-dashboard/ {
		  set $upstream "kubernetes-dashboard.syseleven-kubernetes-dashboard.svc.cluster.local:80";
		  rewrite ^/kubernetes-dashboard(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
	  location = /netdata {
		  return 302 https://$server_name/netdata/;
	  }
	  location /netdata/ {
		  set $upstream "netdata.syseleven-netdata.svc.cluster.local:19999";
		  rewrite ^/netdata(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
	  location = /ui {
		  return 302 https://$server_name/ui/;
	  }
	  location /ui/ {
		  set $upstream "syseleven-vault-ui.syseleven-vault.svc.cluster.local:8200";
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
	  location = /v1 {
		  return 302 https://$server_name/v1/;
	  }
	  location /v1/ {
		  set $upstream "syseleven-vault-ui.syseleven-vault.svc.cluster.local:8200";
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
		  proxy_set_header Upgrade $http_upgrade;
		  proxy_set_header Connection $connection_upgrade;
	  }
	  location = /rabbitmq {
		  return 302 https://$server_name/rabbitmq/;
	  }
	  location /rabbitmq/ {
		  set $upstream "syseleven-rabbitmq.syseleven-rabbitmq.svc.cluster.local:15672";
		  rewrite ^/rabbitmq(/.*) $1 break;
		  proxy_pass http://$upstream$uri$is_args$args;
		  proxy_http_version 1.1;
		  proxy_pass_request_headers on;
	  }
  }
`

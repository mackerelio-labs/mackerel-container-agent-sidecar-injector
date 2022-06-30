# mackerel-container-agent-sidecar-injector

[![exectute test](https://github.com/mackerelio-labs/mackerel-container-agent-sidecar-injector/actions/workflows/ci.yaml/badge.svg)](https://github.com/mackerelio-labs/mackerel-container-agent-sidecar-injector/actions/workflows/ci.yaml)

`mackerel-container-agent Sidecar Injector` allows to dynamically inject [mackerel-container-agent](https://github.com/mackerelio/mackerel-container-agent) as a sidecar container.

## Usage

pre requirements.

```console
kubectl apply --validate=false -f https://github.com/jetstack/cert-manager/releases/latest/download/cert-manager.yaml
```

deploy mackerel-container-agent-sidecar-injector.

```console
export IMG=image-registry.example.com/owner/mackerel-container-agent-sidecar-injector:latest
make docker-build
make docker-push
make deploy
```

use Helm Chart.

```console
export IMG=image-registry.example.com/owner/mackerel-container-agent-sidecar-injector:latest
make docker-build
make docker-push
make helm-deploy
```

### Inject mackerel-container-agent into pod

create ServiceAccount for mackerel-container-agent.

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: default-with-mackerel-agent
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: mackerel-agent-clusterrolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: mackerel-sidecar-injector-container-agent-clusterrole
subjects:
- kind: ServiceAccount
  name: default-with-mackerel-agent
  namespace: default
```

create pod with annotation(`agent-injector.contrib.mackerel.io/inject: true`, `agent-injector.contrib.mackerel.io/mackerel_apikey.secret_name: "mysecret"`) and `ServiceAccount`(default-with-mackerel-agent) created above.

```yaml
---
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: mackerel-example-deployment
spec:
  selector:
    matchLabels:
      app: mackerel-example-app
  template:
    metadata:
      annotations:
        agent-injector.contrib.mackerel.io/inject: "true"
        agent-injector.contrib.mackerel.io/roles: "mackerel:example-app"
        agent-injector.contrib.mackerel.io/mackerel_apikey.secret_name: "mackerel-api-key"
        agent-injector.contrib.mackerel.io/mackere_agent_config.configmap_name: "mackerel-agent-config"
      labels:
        app: mackerel-example-app
    spec:
      serviceAccountName: default-with-mackerel-agent
      containers:
        - name: nginx
          image: nginx:latest
          volumeMounts:
            - name: nginx-status-config
              mountPath: /etc/nginx/conf.d/nginx-status.conf
              subPath: nginx-status.conf
      volumes:
        - name: nginx-status-config
          configMap:
            name: nginx-status-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-status-config
data:
  nginx-status.conf: |
    server{
        listen 8080;
        server_name localhost;
        location /nginx_status {
            stub_status on;
            access_log off;
            allow 127.0.0.1;
            deny all;
        }
    }
---
apiVersion: v1
kind: Secret
metadata:
  name: mackerel-api-key
data:
  MACKEREL_APIKEY: BASE64_DECODED_YOUR_MACKEREL_APIKEY
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: mackerel-agent-config
data:
  mackerel-agent.conf: |
    plugin:
      metrics:
        nginx:
          command: "/usr/bin/mackerel-plugin-nginx"
```

# mackerel-container-agent-sidecar-injector

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
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: mackerel-container-agent-clusterrole
rules:
- apiGroups:
  - ""
  resources:
  - nodes/proxy
  - nodes/stats
  - nodes/spec
  verbs:
  - get
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
  name: mackerel-container-agent-clusterrole
subjects:
- kind: ServiceAccount
  name: default-with-mackerel-agent
  namespace: default
```


create pod with annotation(`agent-injector.contrib.mackerel.io/inject: true`) and `ServiceAccount`(default-with-mackerel-agent) created above.

```yaml
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
      labels:
        app: mackerel-example-app
    spec:
      serviceAccountName: default-with-mackerel-agent
      containers:
        - name: sleep
          image: buildpack-deps:curl
          command: [ "sh", "-c", "while :; do sleep 100; done" ]
```

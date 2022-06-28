# mackerel-container-agent-sidecar-injector

`mackerel-container-agent Sidecar Injector` allows to dynamically inject mackerel-container-agent as a sidecar container.

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

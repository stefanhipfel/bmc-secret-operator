# BMC Secret Operator Helm Chart

A Kubernetes operator that synchronizes BMC (Baseboard Management Controller) credentials to secret backends like Vault or OpenBao.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.0+
- cert-manager (optional, required if `certManager.enable=true`)
- prometheus-operator (optional, required if `prometheus.enable=true`)

## Installation

### Add the Helm repository (if published)

```bash
helm repo add bmc-secret-operator https://charts.example.com
helm repo update
```

### Install the chart

```bash
# Install with default values
helm install bmc-secret-operator ./dist/chart

# Install with custom values
helm install bmc-secret-operator ./dist/chart \
  --namespace bmc-system \
  --create-namespace \
  -f custom-values.yaml
```

### Install with inline configuration

```bash
helm install bmc-secret-operator ./dist/chart \
  --set manager.image.repository=ghcr.io/ironcore-dev/bmc-secret-operator \
  --set manager.image.tag=v0.1.0 \
  --set prometheus.enable=true
```

## Configuration

### Backend Configuration

The operator supports two configuration methods for secret backends:

#### Method 1: Environment Variables (values.yaml)

Configure via Helm values:

```yaml
manager:
  env:
    - name: SECRET_BACKEND_TYPE
      value: vault
    - name: VAULT_ADDR
      value: https://vault.example.com:8200
    - name: VAULT_AUTH_METHOD
      value: kubernetes
    - name: VAULT_ROLE
      value: bmc-secret-operator
    - name: VAULT_MOUNT_PATH
      value: secret
    - name: PATH_TEMPLATE
      value: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
```

#### Method 2: SecretBackendConfig CRD (Recommended)

Create a `SecretBackendConfig` resource after installation:

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: vault-config
spec:
  backendType: vault
  vault:
    address: https://vault.example.com:8200
    authMethod: kubernetes
    role: bmc-secret-operator
    mountPath: secret
    pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
```

The CRD method is recommended because:
- Configuration is stored as a Kubernetes resource
- Changes are applied dynamically without pod restarts
- Better integration with GitOps workflows
- Validation at the API level

### Manager Configuration

```yaml
manager:
  replicas: 1  # Number of controller replicas

  image:
    repository: ghcr.io/ironcore-dev/bmc-secret-operator
    tag: v0.1.0
    pullPolicy: IfNotPresent

  resources:
    limits:
      cpu: 500m
      memory: 128Mi
    requests:
      cpu: 10m
      memory: 64Mi
```

### Metrics Configuration

Enable Prometheus metrics collection:

```yaml
metrics:
  enable: true  # Enable metrics endpoint
  port: 8443    # Metrics server port

prometheus:
  enable: true  # Create ServiceMonitor for Prometheus Operator
```

Available metrics include:
- `bmcsecret_reconcile_duration_seconds` - Reconciliation timing
- `bmcsecret_backend_operation_duration_seconds` - Backend operation latency
- `bmcsecret_sync_success_paths` - Successful syncs per secret
- `bmcsecret_backend_auth_duration_seconds` - Authentication timing

See [metrics documentation](../../docs/metrics.md) for complete list.

### RBAC Helpers

Enable convenience RBAC roles for managing custom resources:

```yaml
rbacHelpers:
  enable: true  # Creates admin/editor/viewer roles
```

### CRD Management

```yaml
crd:
  enable: true  # Install CRDs with the chart
  keep: true    # Keep CRDs when uninstalling
```

### Security Context

The operator runs with secure defaults:

```yaml
manager:
  podSecurityContext:
    runAsNonRoot: true
    seccompProfile:
      type: RuntimeDefault

  securityContext:
    allowPrivilegeEscalation: false
    capabilities:
      drop:
        - ALL
    readOnlyRootFilesystem: true
```

## Usage Examples

### Basic Vault Setup

1. Install the operator:

```bash
helm install bmc-secret-operator ./dist/chart \
  --namespace bmc-system \
  --create-namespace
```

2. Create a SecretBackendConfig:

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: vault-config
spec:
  backendType: vault
  vault:
    address: https://vault.example.com:8200
    authMethod: kubernetes
    role: bmc-secret-operator
    mountPath: secret
    pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
```

3. Create a BMCSecret:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  name: datacenter-bmcs
spec:
  secretRef:
    name: bmc-credentials  # Secret containing BMC credentials
  bmcLabelSelector:
    matchLabels:
      region: us-west-2
```

### With Prometheus Monitoring

```bash
helm install bmc-secret-operator ./dist/chart \
  --set prometheus.enable=true \
  --set certManager.enable=true
```

### With Custom Resource Limits

```bash
helm install bmc-secret-operator ./dist/chart \
  --set manager.resources.limits.cpu=1000m \
  --set manager.resources.limits.memory=256Mi
```

## Upgrading

### Upgrade the chart

```bash
helm upgrade bmc-secret-operator ./dist/chart \
  --namespace bmc-system
```

### Upgrade with new values

```bash
helm upgrade bmc-secret-operator ./dist/chart \
  --namespace bmc-system \
  -f new-values.yaml
```

### View current values

```bash
helm get values bmc-secret-operator -n bmc-system
```

## Uninstallation

```bash
# Uninstall the release (CRDs are kept by default)
helm uninstall bmc-secret-operator --namespace bmc-system

# Remove CRDs manually if needed
kubectl delete crd bmcsecrets.metal.ironcore.dev
kubectl delete crd bmcsecretsyncstatuses.config.metal.ironcore.dev
kubectl delete crd secretbackendconfigs.config.metal.ironcore.dev
```

## Configuration Reference

### Manager Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `manager.replicas` | Number of controller replicas | `1` |
| `manager.image.repository` | Controller image repository | `controller` |
| `manager.image.tag` | Controller image tag | `latest` |
| `manager.image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `manager.args` | Controller arguments | `["--leader-elect", "--health-probe-bind-address=:8081"]` |
| `manager.env` | Environment variables for backend config | `[]` |
| `manager.imagePullSecrets` | Image pull secrets | `[]` |
| `manager.resources` | CPU/Memory resource requests/limits | See values.yaml |
| `manager.affinity` | Pod affinity rules | `{}` |
| `manager.nodeSelector` | Node selector | `{}` |
| `manager.tolerations` | Pod tolerations | `[]` |

### Metrics Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `metrics.enable` | Enable metrics endpoint | `true` |
| `metrics.port` | Metrics server port | `8443` |

### Feature Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `rbacHelpers.enable` | Install convenience RBAC roles | `false` |
| `crd.enable` | Install CRDs with chart | `true` |
| `crd.keep` | Keep CRDs on uninstall | `true` |
| `certManager.enable` | Enable cert-manager integration | `false` |
| `prometheus.enable` | Create ServiceMonitor | `false` |

### Global Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `nameOverride` | Override chart name | `""` |
| `fullnameOverride` | Override full name | `""` |

## Troubleshooting

### Check operator logs

```bash
kubectl logs -n bmc-system -l control-plane=controller-manager -f
```

### Verify CRDs are installed

```bash
kubectl get crd | grep metal.ironcore.dev
```

### Check metrics endpoint

```bash
kubectl port-forward -n bmc-system svc/bmc-secret-operator-metrics 8443:8443
curl -k https://localhost:8443/metrics | grep bmcsecret_
```

### Verify backend connectivity

Check SecretBackendConfig status:

```bash
kubectl get secretbackendconfig -o yaml
```

Check operator events:

```bash
kubectl get events -n bmc-system --sort-by='.lastTimestamp'
```

## Development

### Testing locally

```bash
# Install with local image
helm install bmc-secret-operator ./dist/chart \
  --set manager.image.repository=localhost:5000/bmc-secret-operator \
  --set manager.image.tag=dev \
  --set manager.image.pullPolicy=Always
```

### Dry run

```bash
helm install bmc-secret-operator ./dist/chart --dry-run --debug
```

### Template rendering

```bash
helm template bmc-secret-operator ./dist/chart
```

## Support

For issues and questions:
- GitHub Issues: https://github.com/ironcore-dev/bmc-secret-operator/issues
- Documentation: [../../docs/](../../docs/)

## License

Apache License 2.0

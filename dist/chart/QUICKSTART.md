# Helm Chart Quick Reference

## Installation Examples

### Minimal installation
```bash
helm install bmc-secret-operator ./dist/chart
```

### With Vault backend configuration
```bash
helm install bmc-secret-operator ./dist/chart \
  --set manager.env[0].name=SECRET_BACKEND_TYPE \
  --set manager.env[0].value=vault \
  --set manager.env[1].name=VAULT_ADDR \
  --set manager.env[1].value=https://vault.example.com:8200 \
  --set manager.env[2].name=VAULT_AUTH_METHOD \
  --set manager.env[2].value=kubernetes \
  --set manager.env[3].name=VAULT_ROLE \
  --set manager.env[3].value=bmc-secret-operator
```

### With Prometheus monitoring
```bash
helm install bmc-secret-operator ./dist/chart \
  --set prometheus.enable=true \
  --set certManager.enable=true
```

### Production configuration
```bash
helm install bmc-secret-operator ./dist/chart \
  --namespace bmc-system \
  --create-namespace \
  --set manager.image.repository=ghcr.io/ironcore-dev/bmc-secret-operator \
  --set manager.image.tag=v0.1.0 \
  --set manager.replicas=2 \
  --set manager.resources.limits.cpu=1000m \
  --set manager.resources.limits.memory=256Mi \
  --set prometheus.enable=true \
  --set rbacHelpers.enable=true
```

## Testing Commands

### Lint the chart
```bash
helm lint ./dist/chart
```

### Dry run with default values
```bash
helm template bmc-secret-operator ./dist/chart
```

### Dry run with custom values
```bash
helm template bmc-secret-operator ./dist/chart -f custom-values.yaml
```

### Show computed values
```bash
helm template bmc-secret-operator ./dist/chart --show-only templates/manager/manager.yaml
```

### Validate without cluster
```bash
helm template bmc-secret-operator ./dist/chart > /tmp/manifests.yaml
kubectl apply --dry-run=client -f /tmp/manifests.yaml
```

## Management Commands

### List installed releases
```bash
helm list -A
```

### Get release status
```bash
helm status bmc-secret-operator -n bmc-system
```

### Get release values
```bash
helm get values bmc-secret-operator -n bmc-system
```

### Get release manifest
```bash
helm get manifest bmc-secret-operator -n bmc-system
```

### Upgrade with new values
```bash
helm upgrade bmc-secret-operator ./dist/chart \
  --namespace bmc-system \
  --reuse-values \
  --set manager.image.tag=v0.2.0
```

### Rollback to previous version
```bash
helm rollback bmc-secret-operator -n bmc-system
```

### Uninstall
```bash
helm uninstall bmc-secret-operator -n bmc-system
```

## Configuration Testing

### Test different configurations

```bash
# Test with CRDs disabled
helm template test ./dist/chart --set crd.enable=false | grep -c "CustomResourceDefinition"

# Test with RBAC helpers enabled
helm template test ./dist/chart --set rbacHelpers.enable=true | grep "secretbackendconfig.*role"

# Test with metrics disabled
helm template test ./dist/chart --set metrics.enable=false | grep "metrics-bind-address"

# Test with cert-manager enabled
helm template test ./dist/chart --set certManager.enable=true | grep -A 5 "volumeMounts:"
```

## Troubleshooting

### Check templating errors
```bash
helm template bmc-secret-operator ./dist/chart --debug
```

### Validate YAML syntax
```bash
helm template bmc-secret-operator ./dist/chart | kubectl apply --dry-run=client -f -
```

### Check specific template
```bash
helm template bmc-secret-operator ./dist/chart --show-only templates/manager/manager.yaml
```

## Resource Summary

With all features enabled, the chart deploys:
- 1 CustomResourceDefinition (SecretBackendConfig)
- 1 ServiceAccount
- 6 ClusterRoles (manager, metrics-auth, leader-election, admin, editor, viewer)
- 2 ClusterRoleBindings (manager, metrics-auth)
- 1 Role (leader-election)
- 1 RoleBinding (leader-election)
- 1 Deployment (controller-manager)
- 1 Service (metrics)
- 1 ServiceMonitor (Prometheus integration)

Total: 15 resources

## Development Workflow

### Update chart after code changes
```bash
# Rebuild manifests with kustomize
make manifests

# Regenerate Helm chart from kustomize
kubebuilder edit --plugins=helm.kubebuilder.io/v2-alpha

# Test the updated chart
helm lint ./dist/chart
helm template test ./dist/chart

# Update chart version
# Edit dist/chart/Chart.yaml and bump version
```

### Package chart for distribution
```bash
helm package ./dist/chart -d ./dist/
```

### Test installation locally
```bash
kind create cluster --name bmc-test
helm install bmc-secret-operator ./dist/chart --namespace bmc-system --create-namespace
kubectl get pods -n bmc-system
```

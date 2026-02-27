# Multi-Team Secret Engine Configuration

## Overview

The BMC Secret Operator supports configuring multiple secret engines within a single Vault backend. This allows different teams or environments to sync BMC credentials to separate Vault mount paths with different path templates and label selectors.

## Use Cases

- **Multi-tenancy**: Different teams can have isolated secret engines with team-specific paths
- **Environment separation**: Separate production, development, and staging BMC credentials
- **Access control**: Each secret engine can have different Vault policies and access controls
- **Path organization**: Different teams can use different path templates for their needs

## Configuration

### VaultConfig SecretEngines Field

Add the `secretEngines` array to your `VaultConfig`:

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: multi-team-config
spec:
  backend: vault
  vaultConfig:
    address: "https://vault.example.com:8200"
    authMethod: kubernetes
    kubernetesAuth:
      role: bmc-secret-operator
    secretEngines:
      - name: team-a-prod
        mountPath: team-a/secret
        pathTemplate: "prod/{{.Region}}/{{.Hostname}}/{{.Username}}"
        syncLabel: "team=a"

      - name: team-b-dev
        mountPath: team-b/secret
        pathTemplate: "dev/{{.Region}}/{{.Hostname}}/{{.Username}}"
        syncLabel: "team=b"
```

### SecretEngineConfig Fields

Each secret engine configuration has the following fields:

- **`name`** (required): A descriptive name for this configuration
  - Must be lowercase alphanumeric with hyphens
  - Length: 1-63 characters
  - Pattern: `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
  - Example: `team-a`, `prod-bmcs`, `shared-infra`

- **`mountPath`** (required): The Vault KV secrets engine mount path
  - Example: `team-a/secret`, `shared/kv`, `critical/secret`

- **`pathTemplate`** (optional): Template for building secret paths within this engine
  - Default: `bmc/{{.Region}}/{{.Hostname}}/{{.Username}}`
  - Variables: `{{.Region}}`, `{{.Hostname}}`, `{{.Username}}`
  - Example: `prod/{{.Region}}/{{.Hostname}}/{{.Username}}`

- **`syncLabel`** (required): Label selector for matching BMCSecrets
  - Format: `key` or `key=value`
  - If only key is specified, any value matches
  - Examples:
    - `team=a` - matches BMCSecrets with label `team=a`
    - `sync-enabled` - matches BMCSecrets with label `sync-enabled` (any value)

## Label Matching Behavior

### Label Selector Formats

1. **Key-Value Match** (`key=value`):
   - BMCSecret must have the exact label with exact value
   - Example: `team=a` matches only `team: a`

2. **Key-Only Match** (`key`):
   - BMCSecret must have the label key with any value
   - Example: `critical` matches `critical: true`, `critical: yes`, `critical: prod`

### Multiple Secret Engines

A BMCSecret can sync to **multiple secret engines** if it matches multiple `syncLabel` selectors:

```yaml
# SecretBackendConfig
secretEngines:
  - name: team-a
    mountPath: team-a/secret
    syncLabel: "team=a"

  - name: all-critical
    mountPath: critical/secret
    syncLabel: "critical"
```

```yaml
# BMCSecret with both labels
apiVersion: bmc.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  labels:
    team: a
    critical: "true"
# This will sync to BOTH team-a/secret AND critical/secret
```

### Global Sync Label

The global `spec.syncLabel` field (if specified) acts as an **additional filter**:

```yaml
spec:
  syncLabel: "bmc-secret-operator.metal.ironcore.dev/sync"
  vaultConfig:
    secretEngines:
      - name: team-a
        syncLabel: "team=a"
```

BMCSecrets must have **both** labels to sync:
- Global label: `bmc-secret-operator.metal.ironcore.dev/sync`
- Engine label: `team=a`

## Examples

### Example 1: Team-Based Isolation

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: team-isolation
spec:
  backend: vault
  vaultConfig:
    address: "https://vault.example.com:8200"
    authMethod: kubernetes
    kubernetesAuth:
      role: bmc-secret-operator
    secretEngines:
      - name: team-alpha
        mountPath: alpha/secret
        pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
        syncLabel: "team=alpha"

      - name: team-beta
        mountPath: beta/secret
        pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
        syncLabel: "team=beta"
```

Create BMCSecrets with team labels:

```yaml
apiVersion: bmc.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  name: alpha-bmc-creds
  labels:
    team: alpha
# Syncs to: alpha/secret/bmc/{region}/{hostname}/{username}
```

### Example 2: Environment Separation

```yaml
secretEngines:
  - name: production
    mountPath: prod/secret
    pathTemplate: "prod/bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
    syncLabel: "environment=production"

  - name: staging
    mountPath: staging/secret
    pathTemplate: "staging/bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
    syncLabel: "environment=staging"

  - name: development
    mountPath: dev/secret
    pathTemplate: "dev/bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
    syncLabel: "environment=development"
```

### Example 3: Mixed Criteria

```yaml
secretEngines:
  # All critical systems (any team)
  - name: critical-all
    mountPath: critical/secret
    pathTemplate: "critical/{{.Region}}/{{.Hostname}}/{{.Username}}"
    syncLabel: "critical"

  # Team A production
  - name: team-a-prod
    mountPath: team-a/prod
    pathTemplate: "prod/{{.Region}}/{{.Hostname}}/{{.Username}}"
    syncLabel: "team=a"

  # Team A development
  - name: team-a-dev
    mountPath: team-a/dev
    pathTemplate: "dev/{{.Region}}/{{.Hostname}}/{{.Username}}"
    syncLabel: "team=a"
```

BMCSecret with multiple labels:
```yaml
metadata:
  labels:
    team: a
    critical: "yes"
    environment: production
# Syncs to both: critical/secret AND team-a/prod
```

## Vault Setup

Each secret engine must be enabled in Vault before use:

```bash
# Enable KV v2 secret engines
vault secrets enable -path=team-a/secret kv-v2
vault secrets enable -path=team-b/secret kv-v2
vault secrets enable -path=critical/secret kv-v2

# Create policies for each team
vault policy write team-a-policy - <<EOF
path "team-a/secret/data/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
EOF

vault policy write team-b-policy - <<EOF
path "team-b/secret/data/*" {
  capabilities = ["create", "read", "update", "delete", "list"]
}
EOF

# Grant operator access to all engines
vault write auth/kubernetes/role/bmc-secret-operator \
  bound_service_account_names=bmc-secret-operator \
  bound_service_account_namespaces=bmc-secret-operator-system \
  policies=team-a-policy,team-b-policy,critical-policy \
  ttl=24h
```

## Migration from Single Engine

If you're currently using a single mount path, you can migrate gradually:

### Before (single engine):
```yaml
spec:
  vaultConfig:
    mountPath: secret
    pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
  syncLabel: "sync-enabled"
```

### After (backward compatible):
```yaml
spec:
  vaultConfig:
    mountPath: secret  # Keep as fallback
    pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
    secretEngines:
      # New team-specific engines
      - name: team-a
        mountPath: team-a/secret
        pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
        syncLabel: "team=a"

      # Default for unlabeled secrets
      - name: default
        mountPath: secret
        pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
        syncLabel: "sync-enabled"
  syncLabel: "sync-enabled"
```

## Monitoring

The operator provides metrics for each secret engine configuration:

```prometheus
# Sync operations per engine
bmcsecret_backend_write_duration_seconds{engine="team-a"}
bmcsecret_backend_write_duration_seconds{engine="team-b"}

# Sync status per engine
bmcsecret_sync_status{engine="team-a", status="success"}
bmcsecret_sync_status{engine="team-b", status="failed"}
```

## Troubleshooting

### Secret Not Syncing

1. **Check label selectors**: Ensure BMCSecret has required labels
   ```bash
   kubectl get bmcsecret <name> -o jsonpath='{.metadata.labels}'
   ```

2. **Verify secret engine config**: Check if label matches any engine
   ```bash
   kubectl get secretbackendconfig -o yaml
   ```

3. **Check operator logs**: Look for sync decisions
   ```bash
   kubectl logs -n bmc-secret-operator-system -l control-plane=controller-manager
   ```

### Vault Permission Errors

Ensure the operator's Vault role has access to all mount paths:

```bash
# Test operator access
vault token lookup
vault list team-a/secret/metadata/
vault list team-b/secret/metadata/
```

### Multiple Syncs

If a BMCSecret matches multiple engines and you see duplicates, this is expected behavior. Each matching engine will receive the secret. To limit this, make label selectors more specific.

## Best Practices

1. **Use specific labels**: Prefer `team=a` over just `team` to avoid accidental matches
2. **Plan mount paths**: Use consistent naming like `{team}/secret` or `{env}/secret`
3. **Test incrementally**: Start with one or two engines before rolling out broadly
4. **Document conventions**: Establish team-wide label naming standards
5. **Monitor metrics**: Watch sync success rates per engine
6. **Vault policies**: Ensure proper RBAC in Vault for each mount path
7. **Backup configuration**: Keep SecretBackendConfig in git for disaster recovery

## See Also

- [SecretBackendConfig API Reference](../api/v1alpha1/secretbackendconfig_types.go)
- [Vault KV Secrets Engine](https://www.vaultproject.io/docs/secrets/kv)
- [Vault Policies](https://www.vaultproject.io/docs/concepts/policies)

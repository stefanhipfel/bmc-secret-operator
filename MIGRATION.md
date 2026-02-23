# Configuration Change Migration Guide

This document explains what happens when you change the `SecretBackendConfig` at runtime and how to handle migrations properly.

## How Configuration Changes Work

The BMC Secret Operator now watches the `SecretBackendConfig` resource for changes. When you update the configuration:

1. **Immediate cache invalidation**: The operator detects the change and invalidates its internal cache
2. **New config on next sync**: The next reconciliation (within 5 minutes) uses the new configuration
3. **Gradual migration**: Secrets are gradually migrated as BMCSecrets are reconciled

## Impact of Different Configuration Changes

### 1. Changing `regionLabelKey`

**Example**: Change from `region` to `datacenter`

```yaml
# Old config
spec:
  regionLabelKey: "region"

# New config
spec:
  regionLabelKey: "datacenter"
```

**What happens**:
- Old paths: `bmc/us-east-1/bmc-server1.example.com/admin` (using BMC label `region: us-east-1`)
- New paths: `bmc/dc-west/bmc-server1.example.com/admin` (using BMC label `datacenter: dc-west`)
- Old secrets remain in Vault at old paths (orphaned)
- New secrets are written to new paths

**Migration steps**:

```bash
# 1. Update the config
kubectl apply -f updated-config.yaml

# 2. Wait for cache invalidation (check logs)
kubectl logs -n bmc-secret-operator-system deployment/bmc-secret-operator-controller-manager --tail=20

# 3. View sync status to see new paths
kubectl get bmcsecretsyncstatuses

# 4. Manually clean up old secrets in Vault
vault kv list secret/bmc/us-east-1/  # List old paths
vault kv delete secret/bmc/us-east-1/bmc-server1.example.com/admin  # Delete each old path
```

### 2. Changing `pathTemplate`

**Example**: Change path structure

```yaml
# Old config
spec:
  pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"

# New config
spec:
  pathTemplate: "infrastructure/{{.Region}}/bmc/{{.Hostname}}"
```

**What happens**:
- Old paths: `bmc/us-east-1/bmc-server1.example.com/admin`
- New paths: `infrastructure/us-east-1/bmc/bmc-server1.example.com`
- Old secrets remain at old paths (orphaned)

**Migration steps**: Same as regionLabelKey change above

### 3. Changing `syncLabel`

**Example**: Change which secrets are synced

```yaml
# Old config
spec:
  syncLabel: "sync-to-vault"

# New config
spec:
  syncLabel: "vault-enabled"
```

**What happens**:
- Only BMCSecrets with the NEW label (`vault-enabled`) will be synced
- Secrets with the OLD label (`sync-to-vault`) will no longer be synced
- Old secrets remain in Vault until BMCSecret is deleted

**Migration steps**:

```bash
# 1. Update BMCSecret labels to use new label
kubectl label bmcsecret my-secret vault-enabled=true sync-to-vault-

# 2. Update the config
kubectl apply -f updated-config.yaml

# 3. Verify secrets are still synced
kubectl get bmcsecretsyncstatuses
```

### 4. Changing Vault Address or Auth Method

**Example**: Migrate from one Vault instance to another

```yaml
# Old config
spec:
  vaultConfig:
    address: "https://vault-old.example.com:8200"

# New config
spec:
  vaultConfig:
    address: "https://vault-new.example.com:8200"
```

**What happens**:
- Operator starts writing to NEW Vault instance
- Secrets in OLD Vault instance remain (not automatically migrated)
- This is effectively a clean migration to new backend

**Migration steps**:

```bash
# Option 1: Automated migration (trigger all reconciliations)
# Delete and recreate all BMCSecretSyncStatus resources to force full re-sync
kubectl delete bmcsecretsyncstatuses --all

# Update config
kubectl apply -f new-vault-config.yaml

# Secrets will be synced to new Vault on next reconciliation

# Option 2: Manual migration (export/import)
# Export from old Vault
vault kv get -format=json secret/bmc/us-east-1/bmc-server1.example.com/admin > backup.json

# Update config
kubectl apply -f new-vault-config.yaml

# Import to new Vault (automatic via operator reconciliation)
# Or manually: vault kv put secret/bmc/us-east-1/bmc-server1.example.com/admin @backup.json
```

## Best Practices

### Before Changing Configuration

1. **Document current state**:
```bash
# List all current sync statuses
kubectl get bmcsecretsyncstatuses -o yaml > sync-status-backup.yaml

# Export Vault paths
vault kv list -format=json secret/bmc/ > vault-paths-backup.json
```

2. **Plan the migration**: Determine which secrets need to be moved and to where

3. **Test in non-production**: Apply config changes in dev/staging first

### After Changing Configuration

1. **Verify cache invalidation**:
```bash
kubectl logs -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager --tail=50 \
  | grep "invalidating cache"
```

2. **Monitor sync status**:
```bash
# Watch for sync status updates
kubectl get bmcsecretsyncstatuses -w

# Check for failures
kubectl get bmcsecretsyncstatuses -o json | jq '.items[] | select(.status.failedPaths > 0)'
```

3. **Clean up orphaned secrets**: If paths changed, manually delete old Vault entries

### Force Immediate Re-sync

To force immediate reconciliation after config change instead of waiting 5 minutes:

```bash
# Option 1: Delete sync status resources (forces re-evaluation)
kubectl delete bmcsecretsyncstatus <secret-name>-sync-status

# Option 2: Add/update annotation on BMCSecret (triggers watch)
kubectl annotate bmcsecret <secret-name> force-sync="$(date +%s)" --overwrite

# Option 3: Restart operator pod (not recommended in production)
kubectl rollout restart -n bmc-secret-operator-system deployment/bmc-secret-operator-controller-manager
```

## Automatic Migration Strategy

The operator uses a **lazy migration** approach:

✅ **Advantages**:
- No downtime
- Gradual rollout
- Can verify each secret individually
- Rollback-friendly

❌ **Disadvantages**:
- Old secrets remain until cleanup
- Migration takes time (up to 5 minutes per reconciliation cycle)
- Manual cleanup required for orphaned paths

## Configuration Change Checklist

When changing configuration:

- [ ] Backup current Vault secrets
- [ ] Document current paths (export BMCSecretSyncStatus resources)
- [ ] Apply new SecretBackendConfig
- [ ] Verify cache invalidation in logs
- [ ] Monitor BMCSecretSyncStatus for new paths
- [ ] Verify secrets in Vault at new paths
- [ ] Clean up old paths in Vault
- [ ] Update monitoring/alerting if paths changed

## Examples

### Example 1: Change Region Label from "region" to "location"

```bash
# 1. Check current state
kubectl get bmcsecretsyncstatus admin-creds-sync-status -o yaml
# Shows: path: bmc/us-east-1/bmc1.example.com/admin

# 2. Update BMC labels
kubectl label bmc bmc1 location=us-east-1 --overwrite

# 3. Update config
kubectl patch secretbackendconfig default-backend-config --type=merge -p '{"spec":{"regionLabelKey":"location"}}'

# 4. Wait for next sync (or force via annotation)
kubectl annotate bmcsecret admin-creds force-sync="$(date +%s)" --overwrite

# 5. Verify new path
kubectl get bmcsecretsyncstatus admin-creds-sync-status -o yaml
# Shows: path: bmc/us-east-1/bmc1.example.com/admin (same because region value didn't change)

# 6. If region VALUE changed (e.g., datacenter: dc-1), clean up old path
vault kv delete secret/bmc/us-east-1/bmc1.example.com/admin
```

### Example 2: Migrate to Simpler Path Template

```bash
# Old: bmc/{{.Region}}/{{.Hostname}}/{{.Username}}
# New: bmc/{{.Hostname}}

# 1. Update config
kubectl patch secretbackendconfig default-backend-config --type=merge \
  -p '{"spec":{"pathTemplate":"bmc/{{.Hostname}}"}}'

# 2. Force re-sync of all secrets
for secret in $(kubectl get bmcsecrets -o name | cut -d/ -f2); do
  kubectl annotate bmcsecret $secret force-sync="$(date +%s)" --overwrite
done

# 3. Wait and verify new paths
kubectl get bmcsecretsyncstatuses -o jsonpath='{range .items[*]}{.spec.bmcSecretRef}{"\n"}{range .status.backendPaths[*]}  {.path}{"\n"}{end}{end}'

# 4. Clean up old region-based paths
vault kv list -format=json secret/bmc/us-east-1/ | jq -r '.[]' | while read path; do
  vault kv delete "secret/bmc/us-east-1/$path"
done
```

## Troubleshooting

### Config change not taking effect

**Symptom**: Updated SecretBackendConfig but operator still uses old values

**Check**:
```bash
# Verify the config was updated
kubectl get secretbackendconfig default-backend-config -o yaml

# Check for cache invalidation in logs
kubectl logs -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager --tail=100 \
  | grep "invalidating cache"
```

**Solution**: If no cache invalidation logged, check if SecretBackendConfig controller is running

### Old secrets not cleaned up

**Symptom**: Old paths still exist in Vault after config change

**Explanation**: This is expected behavior - the operator uses lazy migration

**Solution**: Manually clean up old paths (see examples above)

### Secrets syncing to wrong paths

**Symptom**: BMCSecretSyncStatus shows unexpected paths

**Check**:
```bash
# Verify BMC labels match new region key
kubectl get bmc <bmc-name> -o yaml | grep -A10 labels

# Verify current config
kubectl get secretbackendconfig default-backend-config -o jsonpath='{.spec.regionLabelKey}'
```

**Solution**: Ensure BMC labels are updated to match new config

## See Also

- [README.md](../README.md) - General operator documentation
- [QUICKSTART.md](../QUICKSTART.md) - Quick start guide
- [config/samples/](../config/samples/) - Example configurations

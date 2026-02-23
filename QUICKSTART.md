# Quick Start Guide

This guide helps you get the BMC Secret Operator running quickly.

## Prerequisites

1. Kubernetes cluster with metal-operator installed
2. HashiCorp Vault instance
3. kubectl configured

## Step 1: Setup Vault

```bash
# Enable KV v2 secrets engine
vault secrets enable -version=2 -path=secret kv

# Create policy
vault policy write bmc-operator - <<EOF
path "secret/data/bmc/*" {
  capabilities = ["create", "read", "update", "delete"]
}
path "secret/metadata/bmc/*" {
  capabilities = ["list", "read", "delete"]
}
EOF

# Enable Kubernetes auth
vault auth enable kubernetes

# Configure Kubernetes auth
vault write auth/kubernetes/config \
    kubernetes_host="https://kubernetes.default.svc:443"

# Create role
vault write auth/kubernetes/role/bmc-secret-operator \
    bound_service_account_names=bmc-secret-operator-controller-manager \
    bound_service_account_namespaces=bmc-secret-operator-system \
    policies=bmc-operator \
    ttl=1h
```

## Step 2: Install Operator

```bash
# Install CRDs
make install

# Build and deploy (update IMG with your registry)
make docker-build docker-push IMG=myregistry.io/bmc-secret-operator:v0.1.0
make deploy IMG=myregistry.io/bmc-secret-operator:v0.1.0
```

## Step 3: Configure Backend

Edit `config/samples/config_v1alpha1_secretbackendconfig.yaml`:

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: default-backend-config
spec:
  backend: vault
  vaultConfig:
    address: "https://vault.yourdomain.com:8200"  # Update this
    authMethod: kubernetes
    kubernetesAuth:
      role: bmc-secret-operator
      path: kubernetes
    mountPath: secret
    tlsConfig:
      skipVerify: false  # Set to true for testing only
  pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
  regionLabelKey: "region"
  syncLabel: "bmc-secret-operator.metal.ironcore.dev/sync"
```

Apply it:

```bash
kubectl apply -f config/samples/config_v1alpha1_secretbackendconfig.yaml
```

## Step 4: Create Test BMCSecret

```bash
# Create BMCSecret with sync label
kubectl apply -f - <<EOF
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  name: test-admin-creds
  labels:
    bmc-secret-operator.metal.ironcore.dev/sync: "true"
data:
  username: YWRtaW4=       # base64: admin
  password: c2VjcmV0MTIz   # base64: secret123
EOF
```

## Step 5: Create Test BMC

```bash
kubectl apply -f - <<EOF
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: test-bmc-server1
  labels:
    region: us-east-1
spec:
  bmcSecretRef:
    name: test-admin-creds
  hostname: bmc-server1.example.com
  protocol: Redfish
EOF
```

## Step 6: Verify Synchronization

Check operator logs:

```bash
kubectl logs -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager --tail=50
```

Check sync status:

```bash
# View all sync statuses
kubectl get bmcsecretsyncstatuses

# View detailed status for the test secret
kubectl get bmcsecretsyncstatus test-admin-creds-sync-status -o yaml
```

Example output:
```yaml
spec:
  bmcSecretRef: test-admin-creds
status:
  totalPaths: 1
  successfulPaths: 1
  failedPaths: 0
  lastSyncAttempt: "2026-02-23T10:30:00Z"
  backendPaths:
  - path: bmc/us-east-1/bmc-server1.example.com/admin
    bmcName: test-bmc-server1
    region: us-east-1
    hostname: bmc-server1.example.com
    username: admin
    lastSyncTime: "2026-02-23T10:30:00Z"
    syncStatus: Success
```

Check events:

```bash
kubectl get events --field-selector involvedObject.kind=BMCSecret
```

Verify in Vault:

```bash
vault kv get secret/bmc/us-east-1/bmc-server1.example.com/admin
```

Expected output:
```
====== Data ======
Key         Value
---         -----
password    secret123
username    admin
```

## Step 7: Test Update

Update the password:

```bash
kubectl patch bmcsecret test-admin-creds --type=merge -p '{"stringData":{"password":"newsecret456"}}'
```

Wait a few seconds and verify in Vault:

```bash
vault kv get secret/bmc/us-east-1/bmc-server1.example.com/admin
```

## Step 8: Test Deletion

Delete the BMCSecret:

```bash
kubectl delete bmcsecret test-admin-creds
```

Verify cleanup in Vault (should return 404):

```bash
vault kv get secret/bmc/us-east-1/bmc-server1.example.com/admin
```

## Troubleshooting

### Operator not syncing secrets

1. Check if BMCSecret has the required sync label:
```bash
kubectl get bmcsecret test-admin-creds -o yaml | grep -A5 labels
```

2. Check operator logs for errors:
```bash
kubectl logs -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager --tail=100
```

3. Verify Vault connectivity:
```bash
kubectl exec -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager -- \
  curl -k https://vault.yourdomain.com:8200/v1/sys/health
```

### Authentication errors

Verify Vault role exists:
```bash
vault read auth/kubernetes/role/bmc-secret-operator
```

Check service account:
```bash
kubectl get sa -n bmc-secret-operator-system bmc-secret-operator-controller-manager
```

### No BMCs found

Ensure BMC references the correct secret name:
```bash
kubectl get bmc -o yaml | grep -A2 bmcSecretRef
```

## Configuration Options

### Sync All Secrets (No Label Filter)

Remove or comment out the `syncLabel` field:

```yaml
spec:
  # syncLabel: ""  # Empty or omitted = sync all secrets
```

### Custom Path Template

```yaml
spec:
  pathTemplate: "infrastructure/{{.Region}}/bmc/{{.Hostname}}"
```

### Custom Region Label

If your BMCs use a different label for region:

```yaml
spec:
  regionLabelKey: "datacenter"  # Instead of "region"
```

## Next Steps

- Add more BMCSecrets and BMCs to test multi-region scenarios
- Set up Prometheus metrics scraping
- Configure alerts for sync failures
- Review Vault audit logs for compliance
- Implement backup and disaster recovery procedures

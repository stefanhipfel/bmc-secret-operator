# BMC Secret Operator

A Kubernetes operator that synchronizes BMC (Baseboard Management Controller) credentials from the metal-operator’s BMCSecret resources to external secret backends like HashiCorp Vault or OpenBao.

## Overview

The BMC Secret Operator watches BMCSecret resources from the [metal-operator](https://github.com/ironcore-dev/metal-operator), discovers associated BMC infrastructure, and maintains synchronized copies in configurable backend systems using logical hierarchical paths (region/hostname/username).

## Features

- **Automatic Synchronization**: Watches BMCSecret resources and syncs credentials to external backends
- **Selective Sync**: Optional label-based filtering to control which BMCSecrets are synced
- **Multi-BMC Support**: Creates separate backend entries for each BMC that shares credentials
- **Flexible Path Construction**: Configurable path templates using region, hostname, and username
- **Pluggable Backend Architecture**: Interface-based design supporting multiple backends
- **HashiCorp Vault Support**: Full support for Vault KV v1 and v2 engines
- **Multiple Auth Methods**: Kubernetes service account auth, token auth, and AppRole (future)
- **Automatic Cleanup**: Removes backend secrets when BMCSecrets are deleted
- **Configuration Options**: CRD-based or environment variable configuration
- **Runtime Config Reload**: Automatically detects and applies SecretBackendConfig changes
- **Sync Status Tracking**: Dedicated CRD tracks synchronization state per BMCSecret

## Architecture

```
BMCSecret (metal-operator)
    └─> BMC Secret Operator watches
        └─> Discovers BMC resources
            └─> Extracts region, hostname, username
                └─> Builds Vault path: bmc/<region>/<hostname>/<username>
                    └─> Syncs credentials to Vault
```

## Installation

### Prerequisites

- Kubernetes cluster (v1.30+)
- [metal-operator](https://github.com/ironcore-dev/metal-operator) v0.3.0+ installed
- HashiCorp Vault server (v1.12.0+) with KV secrets engine enabled
- Go 1.25.6+ (for building from source)

### Install CRDs

```bash
make install
```

### Deploy Operator

```bash
# Build and push image
make docker-build docker-push IMG=<your-registry>/bmc-secret-operator:latest

# Deploy to cluster
make deploy IMG=<your-registry>/bmc-secret-operator:latest
```

## Configuration

### Option 1: SecretBackendConfig CRD (Recommended)

Create a `SecretBackendConfig` resource:

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: default-backend-config
spec:
  backend: vault
  vaultConfig:
    address: "https://vault.example.com:8200"
    authMethod: kubernetes
    kubernetesAuth:
      role: bmc-secret-operator
      path: kubernetes
    mountPath: secret
    tlsConfig:
      skipVerify: false
  pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
  regionLabelKey: "region"
  # Optional: Only sync BMCSecrets with this label
  syncLabel: "bmc-secret-operator.metal.ironcore.dev/sync"
```

Apply the configuration:

```bash
kubectl apply -f config/samples/config_v1alpha1_secretbackendconfig.yaml
```

**Runtime Configuration Changes**: The operator watches the `SecretBackendConfig` resource and automatically detects changes. When you update the configuration (e.g., change `regionLabelKey` or `pathTemplate`), the operator invalidates its cache and applies the new configuration on the next reconciliation cycle (within 5 minutes). See [MIGRATION.md](./MIGRATION.md) for details on handling configuration changes and migrating secrets.

### Selective Sync with Labels

If you configure a `syncLabel`, only BMCSecrets with that label will be synced:

```yaml
spec:
  syncLabel: "bmc-secret-operator.metal.ironcore.dev/sync"
```

Then label BMCSecrets you want to sync:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  name: admin-creds
  labels:
    bmc-secret-operator.metal.ironcore.dev/sync: "true"
data:
  username: YWRtaW4=
  password: c2VjcmV0MTIz
```

If `syncLabel` is not configured or empty, all BMCSecrets will be synced.

### Option 2: Environment Variables (Fallback)

If no `SecretBackendConfig` is found, the operator falls back to environment variables:

```yaml
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
- name: REGION_LABEL_KEY
  value: region
- name: SYNC_LABEL
  value: "bmc-secret-operator.metal.ironcore.dev/sync"
```

## Vault Setup

### Enable KV v2 Engine

```bash
vault secrets enable -version=2 -path=secret kv
```

### Create Policy

```bash
vault policy write bmc-operator - <<EOF
path "secret/data/bmc/*" {
  capabilities = ["create", "read", "update", "delete"]
}
path "secret/metadata/bmc/*" {
  capabilities = ["list", "read", "delete"]
}
EOF
```

### Configure Kubernetes Auth

```bash
# Enable Kubernetes auth
vault auth enable kubernetes

# Configure auth method
vault write auth/kubernetes/config \
    kubernetes_host="https://kubernetes.default.svc:443"

# Create role
vault write auth/kubernetes/role/bmc-secret-operator \
    bound_service_account_names=bmc-secret-operator-controller-manager \
    bound_service_account_namespaces=bmc-secret-operator-system \
    policies=bmc-operator \
    ttl=1h
```

## Usage

### Example: Syncing BMC Credentials

1. Create a BMCSecret (from metal-operator):

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  name: admin-creds
data:
  username: YWRtaW4=  # base64: admin
  password: c2VjcmV0MTIz  # base64: secret123
```

2. Create BMC resources that reference the secret:

```yaml
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: bmc-us-east-1-server1
  labels:
    region: us-east-1
spec:
  bmcSecretRef:
    name: admin-creds
  hostname: bmc-server1.east.example.com
  protocol: Redfish
```

3. The operator will automatically:
   - Discover the BMC resources referencing `admin-creds`
   - Extract region from labels (`us-east-1`)
   - Extract hostname from spec (`bmc-server1.east.example.com`)
   - Extract username from secret (`admin`)
   - Build Vault path: `bmc/us-east-1/bmc-server1.east.example.com/admin`
   - Sync credentials to Vault

4. Verify in Vault:

```bash
vault kv get secret/bmc/us-east-1/bmc-server1.east.example.com/admin
```

### Monitoring Sync Status

The operator automatically creates a `BMCSecretSyncStatus` resource for each BMCSecret to track synchronization state.

View all sync statuses:

```bash
kubectl get bmcsecretsyncstatuses
```

Example output:
```
NAME                           BMCSECRET      TOTAL   SUCCESSFUL   FAILED   LAST SYNC
admin-creds-sync-status        admin-creds    2       2            0        2026-02-23T10:30:00Z
```

View detailed status for a specific secret:

```bash
kubectl get bmcsecretsyncstatus admin-creds-sync-status -o yaml
```

Example detailed output:

```yaml
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: BMCSecretSyncStatus
metadata:
  name: admin-creds-sync-status
spec:
  bmcSecretRef: admin-creds
status:
  totalPaths: 2
  successfulPaths: 2
  failedPaths: 0
  lastSyncAttempt: "2026-02-23T10:30:00Z"
  backendPaths:
  - path: bmc/us-east-1/bmc-server1.east.example.com/admin
    bmcName: bmc-us-east-1-server1
    region: us-east-1
    hostname: bmc-server1.east.example.com
    username: admin
    lastSyncTime: "2026-02-23T10:30:00Z"
    syncStatus: Success
  - path: bmc/us-west-1/bmc-server5.west.example.com/admin
    bmcName: bmc-us-west-1-server5
    region: us-west-1
    hostname: bmc-server5.west.example.com
    username: admin
    lastSyncTime: "2026-02-23T10:30:00Z"
    syncStatus: Success
  conditions:
  - type: Synced
    status: "True"
    observedGeneration: 1
    lastTransitionTime: "2026-02-23T10:30:00Z"
    reason: AllPathsSynced
    message: Successfully synced to 2 backend paths
```

**Status Fields**:
- `totalPaths`: Number of backend paths that should be synced
- `successfulPaths`: Number of paths successfully synced
- `failedPaths`: Number of paths that failed to sync
- `lastSyncAttempt`: Timestamp of the last reconciliation
- `backendPaths[]`: Detailed information for each backend path
  - `path`: Full path in the backend
  - `bmcName`: Name of the BMC resource
  - `region`, `hostname`, `username`: Path components
  - `lastSyncTime`: When this specific path was last synced
  - `syncStatus`: "Success" or "Failed"
  - `errorMessage`: Error details if sync failed
- `conditions[]`: Kubernetes standard conditions

Watch sync status in real-time:

```bash
kubectl get bmcsecretsyncstatuses -w
```

### Path Template Variables

The operator supports the following variables in path templates:

- `{{.Region}}`: Extracted from BMC labels using `regionLabelKey` (default: "region")
- `{{.Hostname}}`: Extracted from BMC `spec.hostname` field, falls back to BMC name
- `{{.Username}}`: Extracted from BMCSecret data

Default template: `bmc/{{.Region}}/{{.Hostname}}/{{.Username}}`

Custom template example:
```yaml
pathTemplate: "infrastructure/bmc/{{.Region}}/{{.Hostname}}"
```

## Authentication Methods

### Kubernetes Auth (Recommended)

Uses the pod’s service account token:

```yaml
vaultConfig:
  authMethod: kubernetes
  kubernetesAuth:
    role: bmc-secret-operator
    path: kubernetes
```

### Token Auth

Uses a pre-configured token from a Kubernetes secret:

```yaml
vaultConfig:
  authMethod: token
  tokenAuth:
    secretRef:
      name: vault-token
      namespace: bmc-secret-operator-system
      key: token
```

Create the token secret:

```bash
kubectl create secret generic vault-token \
  --from-literal=token=hvs.CAESI... \
  -n bmc-secret-operator-system
```

## Monitoring

The operator emits Kubernetes events:

- `Normal/Synced`: Successfully synced to backend
- `Warning/PartialSync`: Some secrets failed to sync
- `Warning/SyncFailed`: Failed to sync specific path
- `Warning/MissingCredentials`: Username or password not found
- `Warning/BackendUnavailable`: Cannot connect to backend
- `Normal/NoBMCReference`: No BMCs reference this secret

View events:

```bash
kubectl get events --field-selector involvedObject.kind=BMCSecret
```

Check operator logs:

```bash
kubectl logs -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager
```

## Development

### Prerequisites

- Go 1.25.6+
- Kubebuilder v4.11+
- Docker (for building images)
- kubectl
- Access to a Kubernetes cluster

### Build

```bash
make build
```

### Run Locally

```bash
# Set environment variables
export VAULT_ADDR=https://vault.example.com:8200
export VAULT_TOKEN=hvs.CAESI...

# Run operator
make run
```

### Run Tests

```bash
make test
```

### Generate Manifests

```bash
make manifests
```

## Project Structure

```
bmc-secret-operator/
├── api/
│   └── v1alpha1/
│       ├── secretbackendconfig_types.go  # Backend configuration CRD
│       └── groupversion_info.go
├── cmd/
│   └── main.go                           # Operator entry point
├── internal/
│   ├── controller/
│   │   ├── bmcsecret_controller.go       # Main reconciliation logic
│   │   └── bmcresolver/
│   │       ├── resolver.go               # BMC discovery utilities
│   │       └── credentials.go            # Credential extraction
│   └── secretbackend/
│       ├── interface.go                  # Backend interface
│       ├── factory.go                    # Backend factory
│       ├── config.go                     # Configuration structures
│       ├── pathbuilder.go                # Path template builder
│       ├── vault/
│       │   ├── vault.go                  # Vault implementation
│       │   └── auth.go                   # Vault authentication
│       └── openbao/
│           └── openbao.go                # OpenBao stub (future)
├── config/
│   ├── crd/                              # CRD manifests
│   ├── rbac/                             # RBAC configuration
│   ├── manager/                          # Manager deployment
│   └── samples/                          # Example configurations
├── Makefile
├── Dockerfile
└── README.md
```

## Security Considerations

- **Never log passwords or tokens** - The operator sanitizes logs to prevent credential leakage
- **TLS by default** - Always use TLS for Vault communication
- **Minimal RBAC** - Operator has read-only access to BMCSecrets
- **Path-restricted policies** - Vault policies limit access to specific paths
- **Audit trail** - Vault audit logs track all secret operations

## Troubleshooting

### Operator won’t start

Check if metal-operator CRDs are installed:

```bash
kubectl get crd bmcsecrets.metal.ironcore.dev
kubectl get crd bmcs.metal.ironcore.dev
```

### Authentication failures

Verify Vault role and service account binding:

```bash
vault read auth/kubernetes/role/bmc-secret-operator
```

Check operator service account:

```bash
kubectl get sa -n bmc-secret-operator-system bmc-secret-operator-controller-manager
```

### Secrets not syncing

Check operator logs for errors:

```bash
kubectl logs -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager --tail=100
```

Verify BMC resources reference the secret:

```bash
kubectl get bmc -o yaml | grep -A2 bmcSecretRef
```

Verify credentials in BMCSecret:

```bash
kubectl get bmcsecret <name> -o yaml
```

### Vault connection issues

Test connectivity from operator pod:

```bash
kubectl exec -n bmc-secret-operator-system \
  deployment/bmc-secret-operator-controller-manager -- \
  curl -k https://vault.example.com:8200/v1/sys/health
```

## Roadmap

- [ ] OpenBao backend implementation
- [ ] AppRole authentication method
- [ ] Status conditions on BMCSecret
- [ ] Metrics and Prometheus integration
- [ ] Webhook validation for SecretBackendConfig
- [ ] Password hash comparison (instead of plaintext)
- [ ] Token renewal for long-running operations
- [ ] Integration tests with testcontainers
- [ ] E2E tests with real Vault instance

## Contributing

Contributions are welcome! Please open an issue or pull request.

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

## Related Projects

- [metal-operator](https://github.com/ironcore-dev/metal-operator) - Kubernetes operator for bare metal management
- [HashiCorp Vault](https://www.vaultproject.io/) - Secrets management solution
- [OpenBao](https://openbao.org/) - Open source Vault fork

# Testing Metrics Locally

This guide shows how to test the Prometheus metrics implementation locally.

## Prerequisites

- Go 1.23+
- Local Kubernetes cluster (kind, minikube, or similar)
- kubectl configured

## Quick Start

### 1. Build the operator

```bash
make build
```

### 2. Run the operator locally

The operator needs to connect to a Kubernetes cluster and requires backend configuration.

```bash
# Option 1: Run with insecure metrics (HTTP)
make run -- --metrics-bind-address=:8080 --metrics-secure=false

# Option 2: Run with secure metrics (HTTPS) - requires certificates
make run -- --metrics-bind-address=:8443
```

### 3. Verify metrics endpoint

In another terminal:

```bash
# Check if metrics are exposed
curl http://localhost:8080/metrics | grep bmcsecret_

# Or use the verification script
./scripts/verify-metrics.sh
```

### 4. Trigger metric collection

Create test resources to trigger reconciliation:

```bash
# Apply SecretBackendConfig (configure vault or use env vars)
kubectl apply -f - <<EOF
apiVersion: config.metal.ironcore.dev/v1alpha1
kind: SecretBackendConfig
metadata:
  name: default-backend-config
spec:
  backend: vault
  vaultConfig:
    address: "http://vault.example.com:8200"
    authMethod: token
    token: "test-token"
    mountPath: "secret"
  pathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}"
  regionLabelKey: "region"
EOF

# Create a BMCSecret
kubectl apply -f - <<EOF
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMCSecret
metadata:
  name: test-secret
data:
  username: YWRtaW4=  # base64: admin
  password: cGFzc3dvcmQxMjM=  # base64: password123
EOF

# Create a BMC that references the secret
kubectl apply -f - <<EOF
apiVersion: metal.ironcore.dev/v1alpha1
kind: BMC
metadata:
  name: test-bmc
  labels:
    region: us-east-1
spec:
  bmcSecretRef:
    name: test-secret
  hostname: bmc1.example.com
  protocol:
    name: Redfish
EOF
```

### 5. Check metrics after reconciliation

```bash
# View all bmcsecret metrics
curl -s http://localhost:8080/metrics | grep '^bmcsecret_' | grep -v '#'

# Check specific metrics
curl -s http://localhost:8080/metrics | grep bmcsecret_reconcile_duration_seconds
curl -s http://localhost:8080/metrics | grep bmcsecret_backend_operation_total
curl -s http://localhost:8080/metrics | grep bmcsecret_sync_success_paths
```

## Expected Output Examples

### Reconciliation Metrics

```
bmcsecret_reconcile_duration_seconds_bucket{operation="reconcile",le="0.01"} 0
bmcsecret_reconcile_duration_seconds_bucket{operation="reconcile",le="0.05"} 1
bmcsecret_reconcile_duration_seconds_bucket{operation="reconcile",le="+Inf"} 1
bmcsecret_reconcile_duration_seconds_sum{operation="reconcile"} 0.042
bmcsecret_reconcile_duration_seconds_count{operation="reconcile"} 1

bmcsecret_reconcile_total{result="success"} 1
bmcsecret_reconcile_total{result="error"} 0
```

### Backend Operation Metrics

```
bmcsecret_backend_operation_duration_seconds_bucket{backend_type="vault",operation="write",le="0.1"} 3
bmcsecret_backend_operation_duration_seconds_sum{backend_type="vault",operation="write"} 0.087
bmcsecret_backend_operation_duration_seconds_count{backend_type="vault",operation="write"} 3

bmcsecret_backend_operation_total{backend_type="vault",operation="write",result="success"} 3
bmcsecret_backend_operation_total{backend_type="vault",operation="read",result="success"} 3
bmcsecret_backend_operation_total{backend_type="vault",operation="exists",result="success"} 3
```

### Sync Status Metrics

```
bmcsecret_sync_success_paths{secret="test-secret"} 1
bmcsecret_sync_failed_paths{secret="test-secret"} 0
bmcsecret_sync_last_success_timestamp{secret="test-secret"} 1740348000

bmcsecret_bmc_count{secret="test-secret"} 1
```

### Authentication Metrics

```
bmcsecret_backend_auth_duration_seconds_bucket{backend_type="vault",method="token",le="0.1"} 1
bmcsecret_backend_auth_duration_seconds_sum{backend_type="vault",method="token"} 0.089
bmcsecret_backend_auth_duration_seconds_count{backend_type="vault",method="token"} 1

bmcsecret_backend_auth_total{backend_type="vault",method="token",result="success"} 1
```

### Discovery Metrics

```
bmcsecret_bmc_discovery_duration_seconds_bucket{secret="test-secret",le="0.01"} 1
bmcsecret_bmc_discovery_duration_seconds_sum{secret="test-secret"} 0.008
bmcsecret_bmc_discovery_duration_seconds_count{secret="test-secret"} 1

bmcsecret_credential_extraction_total{result="success",secret="test-secret"} 1
```

## Troubleshooting

### Metrics endpoint returns 404

Check the metrics bind address:
```bash
# The operator defaults to disabled metrics (address "0")
# You must explicitly set --metrics-bind-address
./bin/manager --metrics-bind-address=:8080 --metrics-secure=false
```

### No metrics appear

1. Verify the operator is running and metrics collector was initialized:
   ```bash
   # Check logs for "metrics collector initialized"
   ```

2. Trigger a reconciliation by creating/updating a BMCSecret

3. Check controller-runtime's built-in metrics are exposed:
   ```bash
   curl http://localhost:8080/metrics | grep controller_runtime
   ```

### Authentication metrics not appearing

Authentication happens once at startup when the backend is first created. To see auth metrics:

1. Restart the operator
2. Trigger reconciliation (which will initialize the backend)
3. Check metrics immediately

### Backend operation metrics show unexpected operations

The operator checks for secret existence before writing, so you'll see:
- 1 `exists` operation per BMC
- 1 `read` operation per BMC (if secret exists)
- 1 `write` operation per BMC (if update needed)

## Integration with Prometheus

### ServiceMonitor Example

```yaml
apiVersion: v1
kind: Service
metadata:
  name: bmc-secret-operator-metrics
  namespace: bmc-secret-operator-system
  labels:
    control-plane: controller-manager
spec:
  ports:
  - name: https
    port: 8443
    protocol: TCP
    targetPort: 8443
  selector:
    control-plane: controller-manager
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: bmc-secret-operator-metrics
  namespace: bmc-secret-operator-system
spec:
  selector:
    matchLabels:
      control-plane: controller-manager
  endpoints:
  - port: https
    scheme: https
    interval: 30s
    bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    tlsConfig:
      insecureSkipVerify: true
```

### Grafana Dashboard Queries

#### Reconciliation Success Rate
```promql
sum(rate(bmcsecret_reconcile_total{result="success"}[5m]))
/
sum(rate(bmcsecret_reconcile_total[5m]))
```

#### Average Backend Latency by Operation
```promql
rate(bmcsecret_backend_operation_duration_seconds_sum[5m])
/
rate(bmcsecret_backend_operation_duration_seconds_count[5m])
```

#### Secrets with Sync Failures
```promql
count(bmcsecret_sync_failed_paths > 0)
```

#### Stale Secrets (no sync in 1 hour)
```promql
(time() - bmcsecret_sync_last_success_timestamp) > 3600
```

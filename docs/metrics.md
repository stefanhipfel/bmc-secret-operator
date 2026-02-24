# BMC Secret Operator Metrics

This document describes the Prometheus metrics exposed by the BMC Secret Operator.

## Metrics Endpoint

The operator exposes metrics at `http://localhost:8080/metrics` (or `:8443` with TLS enabled).

## Available Metrics

### Reconciliation Metrics

#### `bmcsecret_reconcile_duration_seconds`
- **Type**: Histogram
- **Labels**: `operation` (reconcile, deletion)
- **Description**: Duration of BMCSecret reconciliation operations in seconds
- **Buckets**: 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0

#### `bmcsecret_reconcile_total`
- **Type**: Counter
- **Labels**: `result` (success, error)
- **Description**: Total number of BMCSecret reconciliation attempts

#### `bmcsecret_bmc_count`
- **Type**: Gauge
- **Labels**: `secret`
- **Description**: Number of BMCs discovered for each BMCSecret

### Sync Status Metrics

#### `bmcsecret_sync_success_paths`
- **Type**: Gauge
- **Labels**: `secret`
- **Description**: Number of successfully synced backend paths per BMCSecret

#### `bmcsecret_sync_failed_paths`
- **Type**: Gauge
- **Labels**: `secret`
- **Description**: Number of failed backend paths per BMCSecret

#### `bmcsecret_sync_last_success_timestamp`
- **Type**: Gauge
- **Labels**: `secret`
- **Description**: Timestamp of last successful sync per BMCSecret (Unix time)

### Backend Operation Metrics

#### `bmcsecret_backend_operation_duration_seconds`
- **Type**: Histogram
- **Labels**: `operation` (write, read, delete, exists), `backend_type` (vault, openbao)
- **Description**: Duration of backend operations in seconds
- **Buckets**: 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5

#### `bmcsecret_backend_operation_total`
- **Type**: Counter
- **Labels**: `operation`, `backend_type`, `result` (success, error)
- **Description**: Total number of backend operations

#### `bmcsecret_backend_errors_total`
- **Type**: Counter
- **Labels**: `operation`, `backend_type`, `error_type` (network, auth, not_found, timeout, config, unknown)
- **Description**: Total number of backend errors by error type

### Authentication Metrics

#### `bmcsecret_backend_auth_duration_seconds`
- **Type**: Histogram
- **Labels**: `method` (kubernetes, token, approle), `backend_type`
- **Description**: Duration of backend authentication operations in seconds
- **Buckets**: 0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0

#### `bmcsecret_backend_auth_total`
- **Type**: Counter
- **Labels**: `method`, `backend_type`, `result` (success, error)
- **Description**: Total number of backend authentication attempts

### Discovery Metrics

#### `bmcsecret_bmc_discovery_duration_seconds`
- **Type**: Histogram
- **Labels**: `secret`
- **Description**: Duration of BMC discovery operations in seconds
- **Buckets**: 0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0

#### `bmcsecret_credential_extraction_total`
- **Type**: Counter
- **Labels**: `secret`, `result` (success, error)
- **Description**: Total number of credential extraction attempts

## Verification

### Local Testing

1. Build and run the operator:
   ```bash
   make build
   make run
   ```

2. Check metrics endpoint:
   ```bash
   curl http://localhost:8080/metrics | grep bmcsecret_
   ```

3. Create test resources to trigger reconciliation:
   ```bash
   kubectl apply -f config/samples/
   ```

4. Verify metrics update:
   ```bash
   curl -s http://localhost:8080/metrics | grep bmcsecret_ | grep -v HELP | grep -v TYPE
   ```

### Prometheus Integration

To scrape metrics with Prometheus, configure a ServiceMonitor:

```yaml
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
    bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    tlsConfig:
      insecureSkipVerify: true
```

### Useful Queries

#### Reconciliation Performance
```promql
# P95 reconciliation latency
histogram_quantile(0.95, rate(bmcsecret_reconcile_duration_seconds_bucket[5m]))

# Reconciliation error rate
rate(bmcsecret_reconcile_total{result="error"}[5m])
```

#### Sync Health
```promql
# Secrets with failures
bmcsecret_sync_failed_paths > 0

# Time since last successful sync
time() - bmcsecret_sync_last_success_timestamp
```

#### Backend Performance
```promql
# P99 backend operation latency by operation
histogram_quantile(0.99, rate(bmcsecret_backend_operation_duration_seconds_bucket[5m])) by (operation)

# Backend error rate by type
rate(bmcsecret_backend_errors_total[5m]) by (error_type)
```

## Error Classification

Backend errors are automatically classified into categories:

- **network**: Connection issues, DNS failures
- **auth**: Authentication/authorization failures
- **not_found**: Resource not found errors
- **timeout**: Operation timeouts, context deadlines
- **config**: Configuration validation errors
- **unknown**: Other unclassified errors

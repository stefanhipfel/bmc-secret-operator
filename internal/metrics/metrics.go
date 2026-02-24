/*
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
*/

package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	resultSuccess = "success"
	resultError   = "error"
)

// Collector holds all Prometheus metrics for the BMC secret operator
type Collector struct {
	// Reconciliation metrics
	reconcileDuration   *prometheus.HistogramVec
	reconcileTotal      *prometheus.CounterVec
	bmcCount            *prometheus.GaugeVec
	syncSuccessPaths    *prometheus.GaugeVec
	syncFailedPaths     *prometheus.GaugeVec
	syncLastSuccessTime *prometheus.GaugeVec

	// Backend operation metrics
	backendOpDuration  *prometheus.HistogramVec
	backendOpTotal     *prometheus.CounterVec
	backendErrorsTotal *prometheus.CounterVec

	// Authentication metrics
	backendAuthDuration *prometheus.HistogramVec
	backendAuthTotal    *prometheus.CounterVec

	// Discovery metrics
	bmcDiscoveryDuration *prometheus.HistogramVec
	credentialExtraction *prometheus.CounterVec
}

// NewCollector creates and registers all metrics
func NewCollector() *Collector {
	return &Collector{
		// Reconciliation duration by operation type
		reconcileDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "bmcsecret_reconcile_duration_seconds",
				Help:    "Duration of BMCSecret reconciliation operations in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0, 10.0},
			},
			[]string{"operation"},
		),

		// Reconciliation attempts by result
		reconcileTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bmcsecret_reconcile_total",
				Help: "Total number of BMCSecret reconciliation attempts",
			},
			[]string{"result"},
		),

		// Number of BMCs discovered per secret
		bmcCount: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "bmcsecret_bmc_count",
				Help: "Number of BMCs discovered for each BMCSecret",
			},
			[]string{"secret"},
		),

		// Successful sync paths per secret
		syncSuccessPaths: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "bmcsecret_sync_success_paths",
				Help: "Number of successfully synced backend paths per BMCSecret",
			},
			[]string{"secret"},
		),

		// Failed sync paths per secret
		syncFailedPaths: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "bmcsecret_sync_failed_paths",
				Help: "Number of failed backend paths per BMCSecret",
			},
			[]string{"secret"},
		),

		// Last successful sync timestamp
		syncLastSuccessTime: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "bmcsecret_sync_last_success_timestamp",
				Help: "Timestamp of last successful sync per BMCSecret (Unix time)",
			},
			[]string{"secret"},
		),

		// Backend operation duration
		backendOpDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "bmcsecret_backend_operation_duration_seconds",
				Help:    "Duration of backend operations in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5},
			},
			[]string{"operation", "backend_type"},
		),

		// Backend operation counts
		backendOpTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bmcsecret_backend_operation_total",
				Help: "Total number of backend operations",
			},
			[]string{"operation", "backend_type", "result"},
		),

		// Backend errors by type
		backendErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bmcsecret_backend_errors_total",
				Help: "Total number of backend errors by error type",
			},
			[]string{"operation", "backend_type", "error_type"},
		),

		// Authentication operation duration
		backendAuthDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "bmcsecret_backend_auth_duration_seconds",
				Help:    "Duration of backend authentication operations in seconds",
				Buckets: []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0},
			},
			[]string{"method", "backend_type"},
		),

		// Authentication attempt counts
		backendAuthTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bmcsecret_backend_auth_total",
				Help: "Total number of backend authentication attempts",
			},
			[]string{"method", "backend_type", "result"},
		),

		// BMC discovery duration
		bmcDiscoveryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "bmcsecret_bmc_discovery_duration_seconds",
				Help:    "Duration of BMC discovery operations in seconds",
				Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
			},
			[]string{"secret"},
		),

		// Credential extraction results
		credentialExtraction: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "bmcsecret_credential_extraction_total",
				Help: "Total number of credential extraction attempts",
			},
			[]string{"secret", "result"},
		),
	}
}

// RecordReconcileDuration records the duration of a reconciliation operation
func (c *Collector) RecordReconcileDuration(operation string, duration time.Duration) {
	c.reconcileDuration.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordReconcileResult records the result of a reconciliation attempt
func (c *Collector) RecordReconcileResult(result string) {
	c.reconcileTotal.WithLabelValues(result).Inc()
}

// RecordBMCCount records the number of BMCs discovered for a secret
func (c *Collector) RecordBMCCount(secret string, count int) {
	c.bmcCount.WithLabelValues(secret).Set(float64(count))
}

// RecordSyncStatus records the sync status for a secret
func (c *Collector) RecordSyncStatus(secret string, successPaths, failedPaths int, syncTime time.Time) {
	c.syncSuccessPaths.WithLabelValues(secret).Set(float64(successPaths))
	c.syncFailedPaths.WithLabelValues(secret).Set(float64(failedPaths))
	if successPaths > 0 && failedPaths == 0 {
		c.syncLastSuccessTime.WithLabelValues(secret).Set(float64(syncTime.Unix()))
	}
}

// RecordBackendOperation records a backend operation duration and result
func (c *Collector) RecordBackendOperation(operation, backendType string, duration time.Duration, err error) {
	c.backendOpDuration.WithLabelValues(operation, backendType).Observe(duration.Seconds())

	result := resultSuccess
	if err != nil {
		result = resultError
		c.recordBackendError(operation, backendType, err)
	}
	c.backendOpTotal.WithLabelValues(operation, backendType, result).Inc()
}

// recordBackendError records backend error details
func (c *Collector) recordBackendError(operation, backendType string, err error) {
	errorType := classifyError(err)
	c.backendErrorsTotal.WithLabelValues(operation, backendType, errorType).Inc()
}

// RecordAuth records authentication operation duration and result
func (c *Collector) RecordAuth(method, backendType string, duration time.Duration, err error) {
	c.backendAuthDuration.WithLabelValues(method, backendType).Observe(duration.Seconds())

	result := resultSuccess
	if err != nil {
		result = resultError
	}
	c.backendAuthTotal.WithLabelValues(method, backendType, result).Inc()
}

// RecordBMCDiscovery records BMC discovery operation duration
func (c *Collector) RecordBMCDiscovery(secret string, duration time.Duration) {
	c.bmcDiscoveryDuration.WithLabelValues(secret).Observe(duration.Seconds())
}

// RecordCredentialExtraction records credential extraction result
func (c *Collector) RecordCredentialExtraction(secret string, err error) {
	result := resultSuccess
	if err != nil {
		result = resultError
	}
	c.credentialExtraction.WithLabelValues(secret, result).Inc()
}

// classifyError categorizes errors for better observability
func classifyError(err error) string {
	if err == nil {
		return "none"
	}

	errStr := err.Error()

	// Network/connectivity errors
	if contains(errStr, "connection refused", "connection reset", "dial tcp", "no such host") {
		return "network"
	}

	// Authentication/authorization errors
	if contains(errStr, "authentication failed", "unauthorized", "permission denied", "forbidden", "invalid token") {
		return "auth"
	}

	// Not found errors
	if contains(errStr, "not found", "does not exist") {
		return "not_found"
	}

	// Timeout errors
	if contains(errStr, "timeout", "deadline exceeded", "context canceled") {
		return "timeout"
	}

	// Configuration errors
	if contains(errStr, "invalid configuration", "missing", "required") {
		return "config"
	}

	// Default to unknown
	return "unknown"
}

// contains checks if any of the substrings are present in the string
func contains(s string, substrings ...string) bool {
	for _, substr := range substrings {
		if len(s) >= len(substr) {
			for i := 0; i <= len(s)-len(substr); i++ {
				if s[i:i+len(substr)] == substr {
					return true
				}
			}
		}
	}
	return false
}

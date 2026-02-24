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
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewCollector(t *testing.T) {
	// Create a new collector
	collector := NewCollector()

	if collector == nil {
		t.Fatal("NewCollector returned nil")
	}

	// Verify all metrics are initialized
	if collector.reconcileDuration == nil {
		t.Error("reconcileDuration not initialized")
	}
	if collector.reconcileTotal == nil {
		t.Error("reconcileTotal not initialized")
	}
	if collector.bmcCount == nil {
		t.Error("bmcCount not initialized")
	}
	if collector.syncSuccessPaths == nil {
		t.Error("syncSuccessPaths not initialized")
	}
	if collector.syncFailedPaths == nil {
		t.Error("syncFailedPaths not initialized")
	}
	if collector.syncLastSuccessTime == nil {
		t.Error("syncLastSuccessTime not initialized")
	}
	if collector.backendOpDuration == nil {
		t.Error("backendOpDuration not initialized")
	}
	if collector.backendOpTotal == nil {
		t.Error("backendOpTotal not initialized")
	}
	if collector.backendErrorsTotal == nil {
		t.Error("backendErrorsTotal not initialized")
	}
	if collector.backendAuthDuration == nil {
		t.Error("backendAuthDuration not initialized")
	}
	if collector.backendAuthTotal == nil {
		t.Error("backendAuthTotal not initialized")
	}
	if collector.bmcDiscoveryDuration == nil {
		t.Error("bmcDiscoveryDuration not initialized")
	}
	if collector.credentialExtraction == nil {
		t.Error("credentialExtraction not initialized")
	}
}

func TestRecordReconcileDuration(t *testing.T) {
	// Create new registry for isolated test
	reg := prometheus.NewRegistry()
	collector := &Collector{
		reconcileDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_reconcile_duration_seconds",
				Help: "Test reconcile duration",
			},
			[]string{"operation"},
		),
	}
	reg.MustRegister(collector.reconcileDuration)

	// Record a duration
	collector.RecordReconcileDuration("reconcile", 1500*time.Millisecond)

	// Verify the metric was recorded
	count := testutil.CollectAndCount(collector.reconcileDuration)
	if count != 1 {
		t.Errorf("Expected 1 metric, got %d", count)
	}
}

func TestRecordReconcileResult(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		reconcileTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_reconcile_total",
				Help: "Test reconcile total",
			},
			[]string{"result"},
		),
	}
	reg.MustRegister(collector.reconcileTotal)

	// Record success and error results
	collector.RecordReconcileResult("success")
	collector.RecordReconcileResult("success")
	collector.RecordReconcileResult("error")

	// Verify counts
	if testutil.CollectAndCount(collector.reconcileTotal) == 0 {
		t.Error("No metrics recorded")
	}
}

func TestRecordBMCCount(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		bmcCount: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_bmc_count",
				Help: "Test BMC count",
			},
			[]string{"secret"},
		),
	}
	reg.MustRegister(collector.bmcCount)

	// Record BMC count
	collector.RecordBMCCount("test-secret", 5)

	// Verify the gauge was set
	if testutil.CollectAndCount(collector.bmcCount) == 0 {
		t.Error("No metrics recorded")
	}
}

func TestRecordSyncStatus(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		syncSuccessPaths: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_sync_success_paths",
				Help: "Test sync success paths",
			},
			[]string{"secret"},
		),
		syncFailedPaths: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_sync_failed_paths",
				Help: "Test sync failed paths",
			},
			[]string{"secret"},
		),
		syncLastSuccessTime: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "test_sync_last_success_timestamp",
				Help: "Test sync last success timestamp",
			},
			[]string{"secret"},
		),
	}
	reg.MustRegister(collector.syncSuccessPaths)
	reg.MustRegister(collector.syncFailedPaths)
	reg.MustRegister(collector.syncLastSuccessTime)

	// Record sync status with some failures
	now := time.Now()
	collector.RecordSyncStatus("test-secret", 3, 1, now)

	// Verify success and failed path metrics
	if testutil.CollectAndCount(collector.syncSuccessPaths) == 0 {
		t.Error("No success path metrics recorded")
	}
	if testutil.CollectAndCount(collector.syncFailedPaths) == 0 {
		t.Error("No failed path metrics recorded")
	}

	// Last success time should NOT be recorded when there are failures
	// Now test fully successful sync
	collector.RecordSyncStatus("test-secret-2", 5, 0, now)

	// Verify last success time is recorded for fully successful sync
	if testutil.CollectAndCount(collector.syncLastSuccessTime) == 0 {
		t.Error("No last success time metrics recorded for fully successful sync")
	}
}

func TestRecordBackendOperation(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		backendOpDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_backend_operation_duration_seconds",
				Help: "Test backend operation duration",
			},
			[]string{"operation", "backend_type"},
		),
		backendOpTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_backend_operation_total",
				Help: "Test backend operation total",
			},
			[]string{"operation", "backend_type", "result"},
		),
		backendErrorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_backend_errors_total",
				Help: "Test backend errors total",
			},
			[]string{"operation", "backend_type", "error_type"},
		),
	}
	reg.MustRegister(collector.backendOpDuration)
	reg.MustRegister(collector.backendOpTotal)
	reg.MustRegister(collector.backendErrorsTotal)

	// Record successful operation
	collector.RecordBackendOperation("write", "vault", 100*time.Millisecond, nil)

	// Record failed operation
	collector.RecordBackendOperation("read", "vault", 50*time.Millisecond, errors.New("connection refused"))

	// Verify metrics
	if testutil.CollectAndCount(collector.backendOpDuration) == 0 {
		t.Error("No duration metrics recorded")
	}
	if testutil.CollectAndCount(collector.backendOpTotal) == 0 {
		t.Error("No operation total metrics recorded")
	}
	if testutil.CollectAndCount(collector.backendErrorsTotal) == 0 {
		t.Error("No error metrics recorded")
	}
}

func TestRecordAuth(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		backendAuthDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_backend_auth_duration_seconds",
				Help: "Test backend auth duration",
			},
			[]string{"method", "backend_type"},
		),
		backendAuthTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_backend_auth_total",
				Help: "Test backend auth total",
			},
			[]string{"method", "backend_type", "result"},
		),
	}
	reg.MustRegister(collector.backendAuthDuration)
	reg.MustRegister(collector.backendAuthTotal)

	// Record successful auth
	collector.RecordAuth("kubernetes", "vault", 200*time.Millisecond, nil)

	// Record failed auth
	collector.RecordAuth("token", "vault", 100*time.Millisecond, errors.New("authentication failed"))

	// Verify metrics
	if testutil.CollectAndCount(collector.backendAuthDuration) == 0 {
		t.Error("No auth duration metrics recorded")
	}
	if testutil.CollectAndCount(collector.backendAuthTotal) == 0 {
		t.Error("No auth total metrics recorded")
	}
}

func TestRecordBMCDiscovery(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		bmcDiscoveryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "test_bmc_discovery_duration_seconds",
				Help: "Test BMC discovery duration",
			},
			[]string{"secret"},
		),
	}
	reg.MustRegister(collector.bmcDiscoveryDuration)

	// Record discovery duration
	collector.RecordBMCDiscovery("test-secret", 250*time.Millisecond)

	// Verify metric
	if testutil.CollectAndCount(collector.bmcDiscoveryDuration) == 0 {
		t.Error("No discovery metrics recorded")
	}
}

func TestRecordCredentialExtraction(t *testing.T) {
	reg := prometheus.NewRegistry()
	collector := &Collector{
		credentialExtraction: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "test_credential_extraction_total",
				Help: "Test credential extraction total",
			},
			[]string{"secret", "result"},
		),
	}
	reg.MustRegister(collector.credentialExtraction)

	// Record successful extraction
	collector.RecordCredentialExtraction("test-secret", nil)

	// Record failed extraction
	collector.RecordCredentialExtraction("bad-secret", errors.New("username not found"))

	// Verify metrics
	if testutil.CollectAndCount(collector.credentialExtraction) == 0 {
		t.Error("No credential extraction metrics recorded")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "none",
		},
		{
			name:     "network error - connection refused",
			err:      errors.New("connection refused"),
			expected: "network",
		},
		{
			name:     "network error - dial tcp",
			err:      errors.New("dial tcp: no route to host"),
			expected: "network",
		},
		{
			name:     "auth error - authentication failed",
			err:      errors.New("authentication failed"),
			expected: "auth",
		},
		{
			name:     "auth error - unauthorized",
			err:      errors.New("unauthorized access"),
			expected: "auth",
		},
		{
			name:     "not found error",
			err:      errors.New("secret not found"),
			expected: "not_found",
		},
		{
			name:     "timeout error",
			err:      errors.New("context deadline exceeded"),
			expected: "timeout",
		},
		{
			name:     "config error",
			err:      errors.New("invalid configuration: missing address"),
			expected: "config",
		},
		{
			name:     "unknown error",
			err:      errors.New("something went wrong"),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := classifyError(tt.err)
			if result != tt.expected {
				t.Errorf("classifyError(%v) = %s, expected %s", tt.err, result, tt.expected)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name       string
		s          string
		substrings []string
		expected   bool
	}{
		{
			name:       "single match",
			s:          "connection refused",
			substrings: []string{"refused"},
			expected:   true,
		},
		{
			name:       "multiple substrings, first matches",
			s:          "authentication failed",
			substrings: []string{"auth", "failed", "error"},
			expected:   true,
		},
		{
			name:       "no match",
			s:          "unknown error",
			substrings: []string{"network", "timeout"},
			expected:   false,
		},
		{
			name:       "empty string",
			s:          "",
			substrings: []string{"test"},
			expected:   false,
		},
		{
			name:       "substring longer than string",
			s:          "err",
			substrings: []string{"error"},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := contains(tt.s, tt.substrings...)
			if result != tt.expected {
				t.Errorf("contains(%q, %v) = %v, expected %v", tt.s, tt.substrings, result, tt.expected)
			}
		})
	}
}

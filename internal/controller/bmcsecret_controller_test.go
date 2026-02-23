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

package controller

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/ironcore-dev/bmc-secret-operator/internal/controller/mock"
)

func TestBMCSecretController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BMCSecret Controller Suite")
}

var _ = Describe("BMCSecret Controller", func() {
	var (
		ctx                context.Context
		mockBackend        *mock.MockBackend
		mockBackendFactory *mock.MockBackendFactory
		reconciler         *BMCSecretReconciler
		recorder           *record.FakeRecorder
		scheme             *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme
		scheme = runtime.NewScheme()
		Expect(metalv1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(configv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Setup mock backend
		mockBackend = mock.NewMockBackend()
		var err error
		mockBackendFactory, err = mock.NewMockBackendFactory(
			mockBackend,
			"bmc/{{.Region}}/{{.Hostname}}/{{.Username}}",
			"region",
			"",
		)
		Expect(err).NotTo(HaveOccurred())

		// Create fake recorder
		recorder = record.NewFakeRecorder(100)
	})

	Context("When reconciling a BMCSecret with BMC references", func() {
		It("Should sync credentials to backend", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc-1",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{
						Name: "test-secret",
					},
					Hostname: &hostname,
					Protocol: metalv1alpha1.Protocol{
						Name: metalv1alpha1.ProtocolNameRedfish,
					},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "test-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(1))
			Expect(mockBackend.WriteSecretCalls[0].Path).To(Equal("bmc/us-east-1/bmc-server1.example.com/admin"))
			Expect(mockBackend.WriteSecretCalls[0].Data["username"]).To(Equal("admin"))
			Expect(mockBackend.WriteSecretCalls[0].Data["password"]).To(Equal("secret123"))
		})

		It("Should sync to multiple paths when multiple BMCs reference the same secret", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "shared-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("shared123"),
				},
			}

			hostname1 := "bmc1.example.com"
			bmc1 := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc-1",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "shared-secret"},
					Hostname:     &hostname1,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			hostname2 := "bmc2.example.com"
			bmc2 := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc-2",
					Labels: map[string]string{
						"region": "us-west-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "shared-secret"},
					Hostname:     &hostname2,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc1, bmc2).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "shared-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(2))

			paths := make([]string, len(mockBackend.WriteSecretCalls))
			for i, call := range mockBackend.WriteSecretCalls {
				paths[i] = call.Path
			}
			Expect(paths).To(ConsistOf(
				"bmc/us-east-1/bmc1.example.com/admin",
				"bmc/us-west-1/bmc2.example.com/admin",
			))
		})

		It("Should skip secrets without required sync label when configured", func() {
			mockBackendFactory.SyncLabel = "sync-enabled"

			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unlabeled-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "unlabeled-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "unlabeled-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(0))
		})

		It("Should sync secrets WITH required sync label", func() {
			mockBackendFactory.SyncLabel = "sync-enabled"

			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "labeled-secret",
					Labels: map[string]string{
						"sync-enabled": "true",
					},
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "labeled-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "labeled-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(1))
		})

		It("Should handle missing BMC references gracefully", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "orphan-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "orphan-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(0))
			Eventually(recorder.Events).Should(Receive(ContainSubstring("NoBMCReference")))
		})

		It("Should handle missing credentials gracefully", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "incomplete-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "incomplete-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "incomplete-secret"},
			})
			Expect(err).To(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(0))
			Eventually(recorder.Events).Should(Receive(ContainSubstring("MissingCredentials")))
		})

		It("Should use fallback hostname when BMC has no hostname field", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fallback-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fallback-bmc-name",
					Labels: map[string]string{
						"region": "us-west-2",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "fallback-secret"},
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "fallback-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(1))
			Expect(mockBackend.WriteSecretCalls[0].Path).To(Equal("bmc/us-west-2/fallback-bmc-name/admin"))
		})

		It("Should use 'unknown' region when BMC has no region label", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-region-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-no-region.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc-no-region",
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "no-region-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "no-region-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(1))
			Expect(mockBackend.WriteSecretCalls[0].Path).To(Equal("bmc/unknown/bmc-no-region.example.com/admin"))
		})

		It("Should handle backend write errors gracefully", func() {
			mockBackend.WriteError = fmt.Errorf("backend write failed")

			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "error-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-error.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc-error",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "error-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "error-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Eventually(recorder.Events).Should(Receive(ContainSubstring("PartialSync")))
		})

		It("Should not update backend if password is unchanged", func() {
			err := mockBackend.WriteSecret(ctx, "bmc/us-east-1/bmc-server1.example.com/admin", map[string]interface{}{
				"username": "admin",
				"password": "secret123",
			})
			Expect(err).NotTo(HaveOccurred())

			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unchanged-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "unchanged-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			initialWrites := mockBackend.GetWriteCallCount()

			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "unchanged-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(initialWrites))
		})

		It("Should update backend when password changes", func() {
			err := mockBackend.WriteSecret(ctx, "bmc/us-east-1/bmc-server1.example.com/admin", map[string]interface{}{
				"username": "admin",
				"password": "oldpassword",
			})
			Expect(err).NotTo(HaveOccurred())

			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "changed-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("newpassword"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "changed-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			initialWrites := mockBackend.GetWriteCallCount()

			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "changed-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(initialWrites + 1))

			data, err := mockBackend.ReadSecret(ctx, "bmc/us-east-1/bmc-server1.example.com/admin")
			Expect(err).NotTo(HaveOccurred())
			Expect(data["password"]).To(Equal("newpassword"))
		})

		It("Should handle nonexistent BMCSecret", func() {
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "nonexistent-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(mockBackend.GetWriteCallCount()).To(Equal(0))
		})

		It("Should create BMCSecretSyncStatus tracking successful sync", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "tracked-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "tracked-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				WithStatusSubresource(&configv1alpha1.BMCSecretSyncStatus{}).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "tracked-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify sync status was created
			syncStatus := &configv1alpha1.BMCSecretSyncStatus{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "tracked-secret-sync-status"}, syncStatus)
			Expect(err).NotTo(HaveOccurred())

			// Verify status content
			Expect(syncStatus.Spec.BMCSecretRef).To(Equal("tracked-secret"))
			Expect(syncStatus.Status.TotalPaths).To(Equal(1))
			Expect(syncStatus.Status.SuccessfulPaths).To(Equal(1))
			Expect(syncStatus.Status.FailedPaths).To(Equal(0))
			Expect(syncStatus.Status.BackendPaths).To(HaveLen(1))
			Expect(syncStatus.Status.BackendPaths[0].Path).To(Equal("bmc/us-east-1/bmc-server1.example.com/admin"))
			Expect(syncStatus.Status.BackendPaths[0].BMCName).To(Equal("test-bmc"))
			Expect(syncStatus.Status.BackendPaths[0].SyncStatus).To(Equal("Success"))
		})

		It("Should update BMCSecretSyncStatus with failure information", func() {
			mockBackend.WriteError = fmt.Errorf("backend connection failed")

			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "failed-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "failed-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(bmcSecret, bmc).
				WithStatusSubresource(&configv1alpha1.BMCSecretSyncStatus{}).
				Build()

			reconciler = &BMCSecretReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: mockBackendFactory,
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "failed-secret"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify sync status shows failure
			syncStatus := &configv1alpha1.BMCSecretSyncStatus{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "failed-secret-sync-status"}, syncStatus)
			Expect(err).NotTo(HaveOccurred())

			Expect(syncStatus.Status.TotalPaths).To(Equal(1))
			Expect(syncStatus.Status.SuccessfulPaths).To(Equal(0))
			Expect(syncStatus.Status.FailedPaths).To(Equal(1))
			Expect(syncStatus.Status.BackendPaths).To(HaveLen(1))
			Expect(syncStatus.Status.BackendPaths[0].SyncStatus).To(Equal("Failed"))
			Expect(syncStatus.Status.BackendPaths[0].ErrorMessage).To(ContainSubstring("backend connection failed"))
		})
	})
})

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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	configv1alpha1 "github.com/ironcore-dev/bmc-secret-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/ironcore-dev/bmc-secret-operator/internal/secretbackend"
)

var _ = Describe("SecretBackendConfig Controller", func() {
	var (
		ctx        context.Context
		reconciler *SecretBackendConfigReconciler
		recorder   *record.FakeRecorder
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Setup scheme
		scheme = runtime.NewScheme()
		Expect(configv1alpha1.AddToScheme(scheme)).To(Succeed())

		// Create fake recorder
		recorder = record.NewFakeRecorder(100)
	})

	Context("When SecretBackendConfig changes", func() {
		It("Should invalidate cache when config is updated", func() {
			backendConfig := &configv1alpha1.SecretBackendConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-backend-config",
				},
				Spec: configv1alpha1.SecretBackendConfigSpec{
					Backend: "vault",
					VaultConfig: &configv1alpha1.VaultConfig{
						Address:    "https://vault.example.com:8200",
						AuthMethod: "kubernetes",
						KubernetesAuth: &configv1alpha1.KubernetesAuthConfig{
							Role: "bmc-operator",
							Path: "kubernetes",
						},
						MountPath: "secret",
					},
					PathTemplate:   "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}",
					RegionLabelKey: "region",
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(backendConfig).
				Build()

			backendFactory, err := secretbackend.NewBackendFactory(k8sClient, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				Expect(backendFactory.Close()).To(Succeed())
			}()

			reconciler = &SecretBackendConfigReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: backendFactory,
			}

			// First reconciliation loads config
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "default-backend-config"},
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify event was emitted
			Eventually(recorder.Events).Should(Receive(ContainSubstring("ConfigReloaded")))
		})

		It("Should handle config deletion gracefully", func() {
			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			backendFactory, err := secretbackend.NewBackendFactory(k8sClient, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				Expect(backendFactory.Close()).To(Succeed())
			}()

			reconciler = &SecretBackendConfigReconciler{
				Client:         k8sClient,
				Scheme:         scheme,
				Recorder:       recorder,
				BackendFactory: backendFactory,
			}

			// Reconcile for non-existent config (simulates deletion)
			_, err = reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{Name: "default-backend-config"},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Cache invalidation behavior", func() {
		It("Should reload config after cache invalidation", func() {
			// This test verifies that after InvalidateCache is called,
			// the next GetRegionLabelKey call fetches fresh config

			backendConfig := &configv1alpha1.SecretBackendConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-backend-config",
				},
				Spec: configv1alpha1.SecretBackendConfigSpec{
					Backend:        "vault",
					RegionLabelKey: "region",
					VaultConfig: &configv1alpha1.VaultConfig{
						Address:    "https://vault.example.com:8200",
						AuthMethod: "kubernetes",
						KubernetesAuth: &configv1alpha1.KubernetesAuthConfig{
							Role: "bmc-operator",
							Path: "kubernetes",
						},
						MountPath: "secret",
					},
					PathTemplate: "bmc/{{.Region}}/{{.Hostname}}/{{.Username}}",
				},
			}

			k8sClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(backendConfig).
				Build()

			backendFactory, err := secretbackend.NewBackendFactory(k8sClient, nil)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				Expect(backendFactory.Close()).To(Succeed())
			}()

			// Load initial config
			regionKey, err := backendFactory.GetRegionLabelKey(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(regionKey).To(Equal("region"))

			// Update the config
			err = k8sClient.Get(ctx, types.NamespacedName{Name: "default-backend-config"}, backendConfig)
			Expect(err).NotTo(HaveOccurred())

			backendConfig.Spec.RegionLabelKey = "datacenter"
			err = k8sClient.Update(ctx, backendConfig)
			Expect(err).NotTo(HaveOccurred())

			// Invalidate cache (simulates what the controller does)
			err = backendFactory.InvalidateCache()
			Expect(err).NotTo(HaveOccurred())

			// Fetch region key again - should get new value
			regionKey, err = backendFactory.GetRegionLabelKey(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(regionKey).To(Equal("datacenter"))
		})
	})
})

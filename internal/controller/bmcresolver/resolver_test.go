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

package bmcresolver

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestBMCResolver(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BMCResolver Suite")
}

var _ = Describe("BMCResolver", func() {
	var (
		ctx       context.Context
		k8sClient client.Client
		scheme    *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(metalv1alpha1.AddToScheme(scheme)).To(Succeed())
		k8sClient = fake.NewClientBuilder().WithScheme(scheme).Build()
	})

	Context("FindBMCsForSecret", func() {
		It("Should find BMCs referencing a secret", func() {
			// Create BMCs with different secret references
			hostname1 := "bmc1.example.com"
			bmc1 := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bmc-1",
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "target-secret"},
					Hostname:     &hostname1,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}
			Expect(k8sClient.Create(ctx, bmc1)).To(Succeed())

			hostname2 := "bmc2.example.com"
			bmc2 := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bmc-2",
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "other-secret"},
					Hostname:     &hostname2,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}
			Expect(k8sClient.Create(ctx, bmc2)).To(Succeed())

			hostname3 := "bmc3.example.com"
			bmc3 := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bmc-3",
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "target-secret"},
					Hostname:     &hostname3,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}
			Expect(k8sClient.Create(ctx, bmc3)).To(Succeed())

			// Find BMCs for target-secret
			bmcs, err := FindBMCsForSecret(ctx, k8sClient, "target-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(bmcs).To(HaveLen(2))

			names := make([]string, len(bmcs))
			for i, bmc := range bmcs {
				names[i] = bmc.Name
			}
			Expect(names).To(ConsistOf("bmc-1", "bmc-3"))
		})

		It("Should return empty list when no BMCs reference the secret", func() {
			hostname := "bmc1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "bmc-1",
				},
				Spec: metalv1alpha1.BMCSpec{
					BMCSecretRef: corev1.LocalObjectReference{Name: "different-secret"},
					Hostname:     &hostname,
					Protocol:     metalv1alpha1.Protocol{Name: metalv1alpha1.ProtocolNameRedfish},
				},
			}
			Expect(k8sClient.Create(ctx, bmc)).To(Succeed())

			bmcs, err := FindBMCsForSecret(ctx, k8sClient, "nonexistent-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(bmcs).To(BeEmpty())
		})
	})

	Context("ExtractRegionFromBMC", func() {
		It("Should extract region from BMC labels", func() {
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"region": "us-east-1",
					},
				},
			}

			region := ExtractRegionFromBMC(bmc, "region")
			Expect(region).To(Equal("us-east-1"))
		})

		It("Should return 'unknown' when region label is missing", func() {
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"environment": "production",
					},
				},
			}

			region := ExtractRegionFromBMC(bmc, "region")
			Expect(region).To(Equal("unknown"))
		})

		It("Should return 'unknown' when BMC has no labels", func() {
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
				},
			}

			region := ExtractRegionFromBMC(bmc, "region")
			Expect(region).To(Equal("unknown"))
		})

		It("Should support custom region label keys", func() {
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
					Labels: map[string]string{
						"datacenter": "dc-west-1",
					},
				},
			}

			region := ExtractRegionFromBMC(bmc, "datacenter")
			Expect(region).To(Equal("dc-west-1"))
		})
	})

	Context("GetHostnameFromBMC", func() {
		It("Should extract hostname from BMC spec", func() {
			hostname := "bmc-server1.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
				},
				Spec: metalv1alpha1.BMCSpec{
					Hostname: &hostname,
				},
			}

			extractedHostname := GetHostnameFromBMC(bmc)
			Expect(extractedHostname).To(Equal("bmc-server1.example.com"))
		})

		It("Should fallback to EndpointRef name when hostname is nil", func() {
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
				},
				Spec: metalv1alpha1.BMCSpec{
					EndpointRef: &corev1.LocalObjectReference{
						Name: "endpoint-1",
					},
				},
			}

			hostname := GetHostnameFromBMC(bmc)
			Expect(hostname).To(Equal("endpoint-1"))
		})

		It("Should fallback to BMC name when hostname and endpoint are missing", func() {
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "fallback-bmc-name",
				},
				Spec: metalv1alpha1.BMCSpec{},
			}

			hostname := GetHostnameFromBMC(bmc)
			Expect(hostname).To(Equal("fallback-bmc-name"))
		})

		It("Should prefer hostname over EndpointRef", func() {
			hostname := "specified-hostname.example.com"
			bmc := &metalv1alpha1.BMC{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bmc",
				},
				Spec: metalv1alpha1.BMCSpec{
					Hostname: &hostname,
					EndpointRef: &corev1.LocalObjectReference{
						Name: "endpoint-1",
					},
				},
			}

			extractedHostname := GetHostnameFromBMC(bmc)
			Expect(extractedHostname).To(Equal("specified-hostname.example.com"))
		})
	})

	Context("ExtractCredentials", func() {
		It("Should extract username and password from BMCSecret", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
					"password": []byte("secret123"),
				},
			}

			username, password, err := ExtractCredentials(bmcSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(username).To(Equal("admin"))
			Expect(password).To(Equal("secret123"))
		})

		It("Should return error when username is missing", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"password": []byte("secret123"),
				},
			}

			_, _, err := ExtractCredentials(bmcSecret)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("username not found"))
		})

		It("Should return error when password is missing", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"username": []byte("admin"),
				},
			}

			_, _, err := ExtractCredentials(bmcSecret)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("password not found"))
		})

		It("Should handle base64 encoded values", func() {
			bmcSecret := &metalv1alpha1.BMCSecret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-secret",
				},
				Data: map[string][]byte{
					"username": []byte("YWRtaW4="),     // This is already bytes, not base64
					"password": []byte("c2VjcmV0MTIz"), // This is already bytes, not base64
				},
			}

			username, password, err := ExtractCredentials(bmcSecret)
			Expect(err).NotTo(HaveOccurred())
			// Data field contains raw bytes, not base64
			Expect(username).To(Equal("YWRtaW4="))
			Expect(password).To(Equal("c2VjcmV0MTIz"))
		})
	})
})

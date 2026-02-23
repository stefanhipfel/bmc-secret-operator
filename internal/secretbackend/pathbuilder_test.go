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

package secretbackend

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestPathBuilder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PathBuilder Suite")
}

var _ = Describe("PathBuilder", func() {
	Context("When building paths from templates", func() {
		It("Should build path with default template", func() {
			builder, err := NewPathBuilder("bmc/{{.Region}}/{{.Hostname}}/{{.Username}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "us-east-1",
				Hostname: "bmc-server1.example.com",
				Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("bmc/us-east-1/bmc-server1.example.com/admin"))
		})

		It("Should build path with custom template", func() {
			builder, err := NewPathBuilder("infrastructure/{{.Region}}/{{.Hostname}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "eu-west-1",
				Hostname: "bmc-server2.example.com",
				Username: "root",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("infrastructure/eu-west-1/bmc-server2.example.com"))
		})

		It("Should build path with minimal template", func() {
			builder, err := NewPathBuilder("{{.Username}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "us-east-1",
				Hostname: "bmc-server1.example.com",
				Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("admin"))
		})

		It("Should handle template with special characters", func() {
			builder, err := NewPathBuilder("bmc-secrets/{{.Region}}_{{.Hostname}}_{{.Username}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "us-east-1",
				Hostname: "bmc1.example.com",
				Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("bmc-secrets/us-east-1_bmc1.example.com_admin"))
		})

		It("Should reject invalid template syntax", func() {
			_, err := NewPathBuilder("bmc/{{.Region}/{{.Hostname}}")
			Expect(err).To(HaveOccurred())
		})

		It("Should handle empty variable values", func() {
			builder, err := NewPathBuilder("bmc/{{.Region}}/{{.Hostname}}/{{.Username}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "",
				Hostname: "bmc-server1.example.com",
				Username: "admin",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("bmc//bmc-server1.example.com/admin"))
		})

		It("Should build path without username", func() {
			builder, err := NewPathBuilder("bmc/{{.Region}}/{{.Hostname}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "us-west-2",
				Hostname: "bmc-server3.example.com",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("bmc/us-west-2/bmc-server3.example.com"))
		})

		It("Should support nested path structures", func() {
			builder, err := NewPathBuilder("infrastructure/datacenters/{{.Region}}/hardware/bmc/{{.Hostname}}/credentials/{{.Username}}")
			Expect(err).NotTo(HaveOccurred())

			path, err := builder.Build(PathVariables{
				Region:   "eu-central-1",
				Hostname: "dc01-bmc05.example.com",
				Username: "ipmi-admin",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(path).To(Equal("infrastructure/datacenters/eu-central-1/hardware/bmc/dc01-bmc05.example.com/credentials/ipmi-admin"))
		})
	})
})

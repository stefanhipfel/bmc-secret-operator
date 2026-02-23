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
	"fmt"

	metalv1alpha1 "github.com/ironcore-dev/metal-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// FindBMCsForSecret returns all BMC resources referencing the given BMCSecret name
func FindBMCsForSecret(ctx context.Context, c client.Client, secretName string) ([]metalv1alpha1.BMC, error) {
	var bmcList metalv1alpha1.BMCList
	if err := c.List(ctx, &bmcList); err != nil {
		return nil, fmt.Errorf("failed to list BMC resources: %w", err)
	}

	var matchingBMCs []metalv1alpha1.BMC
	for _, bmc := range bmcList.Items {
		if bmc.Spec.BMCSecretRef.Name == secretName {
			matchingBMCs = append(matchingBMCs, bmc)
		}
	}

	return matchingBMCs, nil
}

// ExtractRegionFromBMC gets the region from BMC labels using configurable key
func ExtractRegionFromBMC(bmc *metalv1alpha1.BMC, regionLabelKey string) string {
	if bmc.Labels == nil {
		return "unknown"
	}

	region, ok := bmc.Labels[regionLabelKey]
	if !ok || region == "" {
		return "unknown"
	}

	return region
}

// GetHostnameFromBMC extracts the hostname field, with fallback to name
func GetHostnameFromBMC(bmc *metalv1alpha1.BMC) string {
	if bmc.Spec.Hostname != nil && *bmc.Spec.Hostname != "" {
		return *bmc.Spec.Hostname
	}

	// Check EndpointRef
	if bmc.Spec.EndpointRef != nil && bmc.Spec.EndpointRef.Name != "" {
		return bmc.Spec.EndpointRef.Name
	}

	// Fallback to BMC name
	return bmc.Name
}

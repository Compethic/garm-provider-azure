// Copyright 2026 Cloudbase Solutions SRL
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package spec

import (
	"encoding/json"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	"github.com/cloudbase/garm-provider-common/params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm-provider-azure/config"
)

func spotBootstrapParams(extraSpecs string) params.BootstrapInstance {
	return params.BootstrapInstance{
		Name:          "test-instance",
		InstanceToken: "test-token",
		OSArch:        params.Amd64,
		OSType:        params.Linux,
		Image:         "Canonical:0001-com-ubuntu-server-jammy:22_04-lts-gen2:latest",
		Flavor:        "Standard_D2ads_v5",
		Tools: []params.RunnerApplicationDownload{
			{
				OS:                to.Ptr("linux"),
				Architecture:      to.Ptr("x64"),
				DownloadURL:       to.Ptr("http://test.com"),
				Filename:          to.Ptr("runner.tar.gz"),
				SHA256Checksum:    to.Ptr("sha256:1123"),
				TempDownloadToken: to.Ptr("test-token"),
			},
		},
		ExtraSpecs: json.RawMessage(extraSpecs),
	}
}

func TestUseSpotMakesAnEvictableCappedSpotVM(t *testing.T) {
	spec, err := GetRunnerSpecFromBootstrapParams(spotBootstrapParams(`{"use_spot": true}`), "controller", &config.Config{})
	require.NoError(t, err)
	require.True(t, spec.UseSpot)

	props, err := spec.GetNewVMProperties("nic-id", VMSizeEphemeralDiskSizeLimits{})
	require.NoError(t, err)

	assert.Equal(t, armcompute.VirtualMachinePriorityTypesSpot, *props.Priority)
	assert.Equal(t, armcompute.VirtualMachineEvictionPolicyTypesDelete, *props.EvictionPolicy)
	assert.Equal(t, float64(-1), *props.BillingProfile.MaxPrice)
}

func TestRegularVMIsTheDefault(t *testing.T) {
	spec, err := GetRunnerSpecFromBootstrapParams(spotBootstrapParams(`{}`), "controller", &config.Config{})
	require.NoError(t, err)

	props, err := spec.GetNewVMProperties("nic-id", VMSizeEphemeralDiskSizeLimits{})
	require.NoError(t, err)

	assert.Nil(t, props.Priority)
	assert.Nil(t, props.EvictionPolicy)
	assert.Nil(t, props.BillingProfile)
}

func TestManagedOSDiskIsDeletedWithTheVM(t *testing.T) {
	spec, err := GetRunnerSpecFromBootstrapParams(spotBootstrapParams(`{}`), "controller", &config.Config{})
	require.NoError(t, err)

	props, err := spec.GetNewVMProperties("nic-id", VMSizeEphemeralDiskSizeLimits{})
	require.NoError(t, err)

	assert.Equal(t, armcompute.DiskDeleteOptionTypesDelete, *props.StorageProfile.OSDisk.DeleteOption)
}

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

package client

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/policy"
	azfake "github.com/Azure/azure-sdk-for-go/sdk/azcore/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5"
	armcomputefake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v5/fake"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5"
	armnetworkfake "github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork/v5/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudbase/garm-provider-azure/config"
)

const sharedRG = "rg-runners"

// deleteCall records one delete request so the tests can assert both the
// resource group it targeted and the order the resources were removed in.
type deleteCall struct {
	kind, resourceGroup, name string
}

type sharedRGFakes struct {
	calls    []deleteCall
	notFound map[string]bool
	fail     map[string]bool
}

func (f *sharedRGFakes) record(kind, rg, name string) (int, azfake.ErrorResponder) {
	f.calls = append(f.calls, deleteCall{kind, rg, name})
	var errResp azfake.ErrorResponder
	switch {
	case f.notFound[kind]:
		errResp.SetResponseError(http.StatusNotFound, "NotFound")
	case f.fail[kind]:
		errResp.SetResponseError(http.StatusConflict, "Conflict")
	}
	return http.StatusOK, errResp
}

func newSharedRGCli(t *testing.T, f *sharedRGFakes) *AzureCli {
	t.Helper()
	opts := func(transport interface {
		Do(*http.Request) (*http.Response, error)
	}) *policy.ClientOptions {
		return &policy.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: transport}}
	}

	vmServer := armcomputefake.VirtualMachinesServer{
		BeginDelete: func(_ context.Context, rg, name string, _ *armcompute.VirtualMachinesClientBeginDeleteOptions) (resp azfake.PollerResponder[armcompute.VirtualMachinesClientDeleteResponse], errResp azfake.ErrorResponder) {
			code, errResp := f.record("vm", rg, name)
			resp.SetTerminalResponse(code, armcompute.VirtualMachinesClientDeleteResponse{}, nil)
			return resp, errResp
		},
		NewListPager: func(rg string, _ *armcompute.VirtualMachinesClientListOptions) (resp azfake.PagerResponder[armcompute.VirtualMachinesClientListResponse]) {
			f.calls = append(f.calls, deleteCall{"list", rg, ""})
			resp.AddPage(http.StatusOK, armcompute.VirtualMachinesClientListResponse{
				VirtualMachineListResult: armcompute.VirtualMachineListResult{
					Value: []*armcompute.VirtualMachine{
						{Name: to.Ptr("runner-1"), Tags: map[string]*string{"garm-pool-id": to.Ptr("pool-1")}},
						{Name: to.Ptr("unrelated"), Tags: map[string]*string{"garm-pool-id": to.Ptr("pool-2")}},
					},
				},
			}, nil)
			return resp
		},
	}
	nicServer := armnetworkfake.InterfacesServer{
		BeginDelete: func(_ context.Context, rg, name string, _ *armnetwork.InterfacesClientBeginDeleteOptions) (resp azfake.PollerResponder[armnetwork.InterfacesClientDeleteResponse], errResp azfake.ErrorResponder) {
			code, errResp := f.record("nic", rg, name)
			resp.SetTerminalResponse(code, armnetwork.InterfacesClientDeleteResponse{}, nil)
			return resp, errResp
		},
	}
	ipServer := armnetworkfake.PublicIPAddressesServer{
		BeginDelete: func(_ context.Context, rg, name string, _ *armnetwork.PublicIPAddressesClientBeginDeleteOptions) (resp azfake.PollerResponder[armnetwork.PublicIPAddressesClientDeleteResponse], errResp azfake.ErrorResponder) {
			code, errResp := f.record("ip", rg, name)
			resp.SetTerminalResponse(code, armnetwork.PublicIPAddressesClientDeleteResponse{}, nil)
			return resp, errResp
		},
	}
	nsgServer := armnetworkfake.SecurityGroupsServer{
		BeginDelete: func(_ context.Context, rg, name string, _ *armnetwork.SecurityGroupsClientBeginDeleteOptions) (resp azfake.PollerResponder[armnetwork.SecurityGroupsClientDeleteResponse], errResp azfake.ErrorResponder) {
			code, errResp := f.record("nsg", rg, name)
			resp.SetTerminalResponse(code, armnetwork.SecurityGroupsClientDeleteResponse{}, nil)
			return resp, errResp
		},
	}
	netServer := armnetworkfake.VirtualNetworksServer{
		BeginDelete: func(_ context.Context, rg, name string, _ *armnetwork.VirtualNetworksClientBeginDeleteOptions) (resp azfake.PollerResponder[armnetwork.VirtualNetworksClientDeleteResponse], errResp azfake.ErrorResponder) {
			code, errResp := f.record("vnet", rg, name)
			resp.SetTerminalResponse(code, armnetwork.VirtualNetworksClientDeleteResponse{}, nil)
			return resp, errResp
		},
	}

	vmCli, err := armcompute.NewVirtualMachinesClient("fake-id", &azfake.TokenCredential{}, opts(armcomputefake.NewVirtualMachinesServerTransport(&vmServer)))
	require.NoError(t, err)
	nicCli, err := armnetwork.NewInterfacesClient("fake-id", &azfake.TokenCredential{}, opts(armnetworkfake.NewInterfacesServerTransport(&nicServer)))
	require.NoError(t, err)
	ipCli, err := armnetwork.NewPublicIPAddressesClient("fake-id", &azfake.TokenCredential{}, opts(armnetworkfake.NewPublicIPAddressesServerTransport(&ipServer)))
	require.NoError(t, err)
	nsgCli, err := armnetwork.NewSecurityGroupsClient("fake-id", &azfake.TokenCredential{}, opts(armnetworkfake.NewSecurityGroupsServerTransport(&nsgServer)))
	require.NoError(t, err)
	netCli, err := armnetwork.NewVirtualNetworksClient("fake-id", &azfake.TokenCredential{}, opts(armnetworkfake.NewVirtualNetworksServerTransport(&netServer)))
	require.NoError(t, err)

	cli := &AzureCli{cfg: &config.Config{ResourceGroup: sharedRG}}
	cli.SetClients(nil, netCli, nil, nsgCli, nicCli, vmCli, ipCli, nil, nil)
	return cli
}

func TestDeleteInstanceResourcesTargetsSharedGroupInOrder(t *testing.T) {
	f := &sharedRGFakes{}
	cli := newSharedRGCli(t, f)

	require.NoError(t, cli.DeleteInstanceResources(context.Background(), "runner-1"))

	assert.Equal(t, []deleteCall{
		{"vm", sharedRG, "runner-1"},
		{"nic", sharedRG, "runner-1"},
		{"ip", sharedRG, "runner-1"},
		{"nsg", sharedRG, "runner-1"},
		{"vnet", sharedRG, "runner-1"},
	}, f.calls)
}

func TestDeleteInstanceResourcesSkipsWhatIsAlreadyGone(t *testing.T) {
	// A spot eviction deletes the VM itself; the rest must still be cleaned up.
	f := &sharedRGFakes{notFound: map[string]bool{"vm": true, "vnet": true}}
	cli := newSharedRGCli(t, f)

	require.NoError(t, cli.DeleteInstanceResources(context.Background(), "runner-1"))
	assert.Len(t, f.calls, 5)
}

func TestDeleteInstanceResourcesStopsOnRealError(t *testing.T) {
	f := &sharedRGFakes{fail: map[string]bool{"nic": true}}
	cli := newSharedRGCli(t, f)

	err := cli.DeleteInstanceResources(context.Background(), "runner-1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete NIC")
	assert.Equal(t, []deleteCall{{"vm", sharedRG, "runner-1"}, {"nic", sharedRG, "runner-1"}}, f.calls)
}

func TestListVirtualMachinesListsOnlyTheSharedGroup(t *testing.T) {
	f := &sharedRGFakes{}
	cli := newSharedRGCli(t, f)

	vms, err := cli.ListVirtualMachines(context.Background(), "pool-1")
	require.NoError(t, err)

	require.Len(t, vms, 1)
	assert.Equal(t, "runner-1", *vms[0].Name)
	assert.Equal(t, []deleteCall{{"list", sharedRG, ""}}, f.calls)
}

func TestResourceGroupFallsBackToOnePerRunner(t *testing.T) {
	assert.Equal(t, "runner-1", (&AzureCli{cfg: &config.Config{}}).resourceGroup("runner-1"))
	assert.Equal(t, sharedRG, (&AzureCli{cfg: &config.Config{ResourceGroup: sharedRG}}).resourceGroup("runner-1"))
}

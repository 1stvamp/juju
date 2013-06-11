// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

type azureInstance struct{}

// azureInstance implements Instance.
var _ environs.Instance = (*azureInstance)(nil)

// Id is specified in the Instance interface.
func (instance *azureInstance) Id() state.InstanceId {
	panic("unimplemented")
}

// DNSName is specified in the Instance interface.
func (instance *azureInstance) DNSName() (string, error) {
	panic("unimplemented")
}

// WaitDNSName is specified in the Instance interface.
func (instance *azureInstance) WaitDNSName() (string, error) {
	panic("unimplemented")
}

// OpenPorts is specified in the Instance interface.
func (instance *azureInstance) OpenPorts(machineId string, ports []params.Port) error {
	panic("unimplemented")
}

// ClosePorts is specified in the Instance interface.
func (instance *azureInstance) ClosePorts(machineId string, ports []params.Port) error {
	panic("unimplemented")
}

// Ports is specified in the Instance interface.
func (instance *azureInstance) Ports(machineId string) ([]params.Port, error) {
	panic("unimplemented")
}
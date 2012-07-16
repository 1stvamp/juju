package main

import (
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/state"
)

type InitzkCommand struct {
	StateInfo  state.Info
	InstanceId string
	EnvType    string
}

// Info returns a decription of the command.
func (c *InitzkCommand) Info() *cmd.Info {
	return &cmd.Info{"initzk", "", "initialize juju state in a local zookeeper", ""}
}

// Init initializes the command for running.
func (c *InitzkCommand) Init(f *gnuflag.FlagSet, args []string) error {
	stateInfoVar(f, &c.StateInfo, "zookeeper-servers", []string{"127.0.0.1:2181"}, "address of zookeeper to initialize")
	f.StringVar(&c.InstanceId, "instance-id", "", "instance id of this machine")
	f.StringVar(&c.EnvType, "env-type", "", "environment type")
	if err := f.Parse(true, args); err != nil {
		return err
	}
	if c.StateInfo.Addrs == nil {
		return requiredError("zookeeper-servers")
	}
	if c.InstanceId == "" {
		return requiredError("instance-id")
	}
	if c.EnvType == "" {
		return requiredError("env-type")
	}
	return cmd.CheckEmpty(f.Args())
}

// Run initializes zookeeper state for an environment.
func (c *InitzkCommand) Run(_ *cmd.Context) error {
	st, err := state.Initialize(&c.StateInfo)
	if err != nil {
		return err
	}
	defer st.Close()

	// manually insert machine/0 into the state
	m, err := st.AddMachine()
	if err != nil {
		return err
	}

	// set the instance id of machine/0 
	if err := m.SetInstanceId(c.InstanceId); err != nil {
		return err
	}
	return nil
}

// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package local_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	coreCloudinit "github.com/juju/juju/cloudinit"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/container"
	"github.com/juju/juju/container/lxc"
	containertesting "github.com/juju/juju/container/testing"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/jujutest"
	envtesting "github.com/juju/juju/environs/testing"
	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/arch"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/provider/local"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	coretesting "github.com/juju/juju/testing"
	coretools "github.com/juju/juju/tools"
	"github.com/juju/juju/version"
)

const echoCommandScript = "#!/bin/sh\necho $0 \"$@\" >> $0.args"

type environSuite struct {
	baseProviderSuite
	envtesting.ToolsFixture
}

var _ = gc.Suite(&environSuite{})

func (s *environSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	s.ToolsFixture.SetUpTest(c)
}

func (s *environSuite) TearDownTest(c *gc.C) {
	s.ToolsFixture.TearDownTest(c)
	s.baseProviderSuite.TearDownTest(c)
}

func (*environSuite) TestOpenFailsWithProtectedDirectories(c *gc.C) {
	testConfig := minimalConfig(c)
	testConfig, err := testConfig.Apply(map[string]interface{}{
		"root-dir": "/usr/lib/juju",
	})
	c.Assert(err, gc.IsNil)

	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.ErrorMatches, "failure setting config: mkdir .* permission denied")
	c.Assert(environ, gc.IsNil)
}

func (s *environSuite) TestNameAndStorage(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(environ.Config().Name(), gc.Equals, "test")
	c.Assert(environ.Storage(), gc.NotNil)
}

func (s *environSuite) TestGetToolsMetadataSources(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	sources, err := tools.GetMetadataSources(environ)
	c.Assert(err, gc.IsNil)
	c.Assert(len(sources), gc.Equals, 1)
	url, err := sources[0].URL("")
	c.Assert(err, gc.IsNil)
	c.Assert(strings.Contains(url, "/tools"), jc.IsTrue)
}

func (*environSuite) TestSupportedArchitectures(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	arches, err := environ.SupportedArchitectures()
	c.Assert(err, gc.IsNil)
	for _, a := range arches {
		c.Assert(arch.IsSupportedArch(a), jc.IsTrue)
	}
}

func (*environSuite) TestSupportNetworks(c *gc.C) {
	testConfig := minimalConfig(c)
	environ, err := local.Provider.Open(testConfig)
	c.Assert(err, gc.IsNil)
	c.Assert(environ.SupportNetworks(), jc.IsFalse)
}

type localJujuTestSuite struct {
	baseProviderSuite
	jujutest.Tests
	oldUpstartLocation string
	testPath           string
	fakesudo           string
}

func (s *localJujuTestSuite) SetUpTest(c *gc.C) {
	s.baseProviderSuite.SetUpTest(c)
	// Construct the directories first.
	err := local.CreateDirs(c, minimalConfig(c))
	c.Assert(err, gc.IsNil)
	s.testPath = c.MkDir()
	s.fakesudo = filepath.Join(s.testPath, "sudo")
	s.PatchEnvPathPrepend(s.testPath)
	s.PatchValue(&lxc.TemplateLockDir, c.MkDir())
	s.PatchValue(&lxc.TemplateStopTimeout, 500*time.Millisecond)

	// Write a fake "sudo" which records its args to sudo.args.
	err = ioutil.WriteFile(s.fakesudo, []byte(echoCommandScript), 0755)
	c.Assert(err, gc.IsNil)

	// Add in an admin secret
	s.Tests.TestConfig["admin-secret"] = "sekrit"
	s.PatchValue(local.CheckIfRoot, func() bool { return false })
	s.Tests.SetUpTest(c)

	s.PatchValue(local.ExecuteCloudConfig, func(environs.BootstrapContext, *cloudinit.MachineConfig, *coreCloudinit.Config) error {
		return nil
	})
}

func (s *localJujuTestSuite) TearDownTest(c *gc.C) {
	s.Tests.TearDownTest(c)
	s.baseProviderSuite.TearDownTest(c)
}

func (s *localJujuTestSuite) MakeTool(c *gc.C, name, script string) {
	path := filepath.Join(s.testPath, name)
	script = "#!/bin/bash\n" + script
	err := ioutil.WriteFile(path, []byte(script), 0755)
	c.Assert(err, gc.IsNil)
}

func (s *localJujuTestSuite) StoppedStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service stop/waiting"`)
}

func (s *localJujuTestSuite) RunningStatus(c *gc.C) {
	s.MakeTool(c, "status", `echo "some-service start/running, process 123"`)
}

var _ = gc.Suite(&localJujuTestSuite{
	Tests: jujutest.Tests{
		TestConfig: minimalConfigValues(),
	},
})

func (s *localJujuTestSuite) TestStartStop(c *gc.C) {
	c.Skip("StartInstance not implemented yet.")
}

func (s *localJujuTestSuite) testBootstrap(c *gc.C, cfg *config.Config) environs.Environ {
	ctx := coretesting.Context(c)
	environ, err := local.Provider.Prepare(ctx, cfg)
	c.Assert(err, gc.IsNil)
	envtesting.UploadFakeTools(c, environ.Storage())
	defer environ.Storage().RemoveAll()
	availableTools := coretools.List{&coretools.Tools{
		Version: version.Current,
		URL:     "http://testing.invalid/tools.tar.gz",
	}}
	_, _, finalizer, err := environ.Bootstrap(ctx, environs.BootstrapParams{
		AvailableTools: availableTools,
	})
	c.Assert(err, gc.IsNil)
	mcfg, err := environs.NewBootstrapMachineConfig(constraints.Value{}, "quantal")
	c.Assert(err, gc.IsNil)
	mcfg.Tools = availableTools[0]
	err = finalizer(ctx, mcfg)
	c.Assert(err, gc.IsNil)
	return environ
}

func (s *localJujuTestSuite) TestBootstrap(c *gc.C) {

	minCfg := minimalConfig(c)

	mockFinish := func(ctx environs.BootstrapContext, mcfg *cloudinit.MachineConfig, cloudcfg *coreCloudinit.Config) error {

		envCfgAttrs := minCfg.AllAttrs()
		if val, ok := envCfgAttrs["enable-os-refresh-update"]; !ok {
			c.Check(cloudcfg.AptUpdate(), gc.Equals, false)
		} else {
			c.Check(cloudcfg.AptUpdate(), gc.Equals, val)
		}

		if val, ok := envCfgAttrs["enable-os-upgrade"]; !ok {
			c.Check(cloudcfg.AptUpgrade(), gc.Equals, false)
		} else {
			c.Check(cloudcfg.AptUpgrade(), gc.Equals, val)
		}

		if !mcfg.EnableOSRefreshUpdate {
			c.Assert(cloudcfg.Packages(), gc.HasLen, 0)
		}
		c.Assert(mcfg.AgentEnvironment, gc.Not(gc.IsNil))
		// local does not allow machine-0 to host units
		c.Assert(mcfg.Jobs, gc.DeepEquals, []params.MachineJob{params.JobManageEnviron})
		return nil
	}
	s.PatchValue(local.ExecuteCloudConfig, mockFinish)

	// Test that defaults are correct.
	s.testBootstrap(c, minCfg)

	// Test that overrides work.
	minCfg, err := minCfg.Apply(map[string]interface{}{
		"enable-os-refresh-update": true,
		"enable-os-upgrade":        true,
	})
	c.Assert(err, gc.IsNil)
	s.testBootstrap(c, minCfg)
}

func (s *localJujuTestSuite) TestDestroy(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	err := env.Destroy()
	// Succeeds because there's no "agents" directory,
	// so destroy will just return without attempting
	// sudo or anything.
	c.Assert(err, gc.IsNil)
	c.Assert(s.fakesudo+".args", jc.DoesNotExist)
}

func (s *localJujuTestSuite) makeAgentsDir(c *gc.C, env environs.Environ) {
	rootDir := env.Config().AllAttrs()["root-dir"].(string)
	agentsDir := filepath.Join(rootDir, "agents")
	err := os.Mkdir(agentsDir, 0755)
	c.Assert(err, gc.IsNil)
}

func (s *localJujuTestSuite) TestDestroyCallSudo(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	s.makeAgentsDir(c, env)
	err := env.Destroy()
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadFile(s.fakesudo + ".args")
	c.Assert(err, gc.IsNil)
	expected := []string{
		s.fakesudo,
		"env",
		"JUJU_HOME=" + osenv.JujuHome(),
		os.Args[0],
		"destroy-environment",
		"-y",
		"--force",
		env.Config().Name(),
	}
	c.Assert(string(data), gc.Equals, strings.Join(expected, " ")+"\n")
}

func (s *localJujuTestSuite) makeFakeUpstartScripts(c *gc.C, env environs.Environ,
) (mongoService *upstart.Service, machineAgent *upstart.Service) {
	upstartDir := c.MkDir()
	s.PatchValue(&upstart.InitDir, upstartDir)
	s.MakeTool(c, "start", `echo "some-service start/running, process 123"`)

	namespace := env.Config().AllAttrs()["namespace"].(string)
	mongoConf := common.Conf{
		Desc: "fake mongo",
		Cmd:  "echo FAKE",
	}
	mongoService = upstart.NewService(mongo.ServiceName(namespace), mongoConf)
	err := mongoService.Install()
	c.Assert(err, gc.IsNil)
	c.Assert(mongoService.Installed(), jc.IsTrue)

	agentConf := common.Conf{
		Desc: "fake agent",
		Cmd:  "echo FAKE",
	}
	machineAgent = upstart.NewService(fmt.Sprintf("juju-agent-%s", namespace), agentConf)

	err = machineAgent.Install()
	c.Assert(err, gc.IsNil)
	c.Assert(machineAgent.Installed(), jc.IsTrue)

	return mongoService, machineAgent
}

func (s *localJujuTestSuite) TestDestroyRemovesUpstartServices(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	s.makeAgentsDir(c, env)
	mongo, machineAgent := s.makeFakeUpstartScripts(c, env)
	s.PatchValue(local.CheckIfRoot, func() bool { return true })

	err := env.Destroy()
	c.Assert(err, gc.IsNil)

	c.Assert(mongo.Installed(), jc.IsFalse)
	c.Assert(machineAgent.Installed(), jc.IsFalse)
}

func (s *localJujuTestSuite) TestDestroyRemovesContainers(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))
	s.makeAgentsDir(c, env)
	s.PatchValue(local.CheckIfRoot, func() bool { return true })

	namespace := env.Config().AllAttrs()["namespace"].(string)
	manager, err := lxc.NewContainerManager(container.ManagerConfig{
		container.ConfigName:   namespace,
		container.ConfigLogDir: "logdir",
		"use-clone":            "false",
	})
	c.Assert(err, gc.IsNil)

	machine1 := containertesting.CreateContainer(c, manager, "1")

	err = env.Destroy()
	c.Assert(err, gc.IsNil)

	container := s.ContainerFactory.New(string(machine1.Id()))
	c.Assert(container.IsConstructed(), jc.IsFalse)
}

func (s *localJujuTestSuite) TestBootstrapRemoveLeftovers(c *gc.C) {
	cfg := minimalConfig(c)
	rootDir := cfg.AllAttrs()["root-dir"].(string)

	// Create a dir inside local/log that should be removed by Bootstrap.
	logThings := filepath.Join(rootDir, "log", "things")
	err := os.MkdirAll(logThings, 0755)
	c.Assert(err, gc.IsNil)

	// Create a cloud-init-output.log in root-dir that should be
	// removed/truncated by Bootstrap.
	cloudInitOutputLog := filepath.Join(rootDir, "cloud-init-output.log")
	err = ioutil.WriteFile(cloudInitOutputLog, []byte("ohai"), 0644)
	c.Assert(err, gc.IsNil)

	s.testBootstrap(c, cfg)
	c.Assert(logThings, jc.DoesNotExist)
	c.Assert(cloudInitOutputLog, jc.DoesNotExist)
	c.Assert(filepath.Join(rootDir, "log"), jc.IsSymlink)
}

func (s *localJujuTestSuite) TestConstraintsValidator(c *gc.C) {
	ctx := coretesting.Context(c)
	env, err := local.Provider.Prepare(ctx, minimalConfig(c))
	c.Assert(err, gc.IsNil)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)
	hostArch := arch.HostArch()
	cons := constraints.MustParse(fmt.Sprintf("arch=%s instance-type=foo tags=bar cpu-power=10 cpu-cores=2", hostArch))
	unsupported, err := validator.Validate(cons)
	c.Assert(err, gc.IsNil)
	c.Assert(unsupported, jc.SameContents, []string{"cpu-cores", "cpu-power", "instance-type", "tags"})
}

func (s *localJujuTestSuite) TestConstraintsValidatorVocab(c *gc.C) {
	env := s.Prepare(c)
	validator, err := env.ConstraintsValidator()
	c.Assert(err, gc.IsNil)

	hostArch := arch.HostArch()
	var invalidArch string
	for _, a := range arch.AllSupportedArches {
		if a != hostArch {
			invalidArch = a
			break
		}
	}
	cons := constraints.MustParse(fmt.Sprintf("arch=%s", invalidArch))
	_, err = validator.Validate(cons)
	c.Assert(err, gc.ErrorMatches, "invalid constraint value: arch="+invalidArch+"\nvalid values are:.*")
}

func (s *localJujuTestSuite) TestStateServerInstances(c *gc.C) {
	env := s.testBootstrap(c, minimalConfig(c))

	instances, err := env.StateServerInstances()
	c.Assert(err, gc.Equals, environs.ErrNotBootstrapped)
	c.Assert(instances, gc.HasLen, 0)

	s.makeAgentsDir(c, env)
	instances, err = env.StateServerInstances()
	c.Assert(err, gc.IsNil)
	c.Assert(instances, gc.DeepEquals, []instance.Id{"localhost"})
}

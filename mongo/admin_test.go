// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service/upstart"
	coretesting "github.com/juju/juju/testing"
)

type adminSuite struct {
	coretesting.BaseSuite
	serviceStarts int
	serviceStops  int
}

var _ = gc.Suite(&adminSuite{})

func (s *adminSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.serviceStarts = 0
	s.serviceStops = 0
	s.PatchValue(mongo.UpstartConfInstall, func(conf *upstart.Service) error {
		return nil
	})
	s.PatchValue(mongo.UpstartServiceStart, func(svc *upstart.Service) error {
		s.serviceStarts++
		return nil
	})
	s.PatchValue(mongo.UpstartServiceStop, func(svc *upstart.Service) error {
		s.serviceStops++
		return nil
	})
}

func (s *adminSuite) TestEnsureAdminUser(c *gc.C) {
	inst := &gitjujutesting.MgoInstance{}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, gc.IsNil)
	defer inst.DestroyWithLog()
	dialInfo := inst.DialInfo()

	// Mock out mongod, so the --noauth execution doesn't
	// do anything nasty. Also mock out the Signal method.
	gitjujutesting.PatchExecutableAsEchoArgs(c, s, "mongod")
	mongodDir := filepath.SplitList(os.Getenv("PATH"))[0]
	s.PatchValue(&mongo.JujuMongodPath, filepath.Join(mongodDir, "mongod"))
	s.PatchValue(mongo.ProcessSignal, func(*os.Process, os.Signal) error {
		return nil
	})

	// First call succeeds, as there are no users yet.
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsTrue)

	// EnsureAdminUser should have stopped the mongo service,
	// started a new mongod with --noauth, and then finally
	// started the service back up.
	c.Assert(s.serviceStarts, gc.Equals, 1)
	c.Assert(s.serviceStops, gc.Equals, 1)
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, gc.IsNil)
	gitjujutesting.AssertEchoArgs(c, "mongod",
		"--noauth",
		"--dbpath", "db",
		"--sslOnNormalPorts",
		"--sslPEMKeyFile", "server.pem",
		"--sslPEMKeyPassword", "ignored",
		"--bind_ip", "127.0.0.1",
		"--port", portString,
		"--noprealloc",
		"--syslog",
		"--smallfiles",
		"--journal",
	)

	// Second call succeeds, as the admin user is already there.
	added, err = s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsFalse)

	// There should have been no additional start/stop.
	c.Assert(s.serviceStarts, gc.Equals, 1)
	c.Assert(s.serviceStops, gc.Equals, 1)
}

func (s *adminSuite) TestEnsureAdminUserError(c *gc.C) {
	inst := &gitjujutesting.MgoInstance{}
	inst.EnableAuth = true
	err := inst.Start(coretesting.Certs)
	c.Assert(err, gc.IsNil)
	defer inst.Destroy()
	dialInfo := inst.DialInfo()

	// First call succeeds, as there are no users yet (mimics --noauth).
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsTrue)

	// Second call fails, as there is another user and the database doesn't
	// actually get reopened with --noauth in the test; mimics AddUser failure
	_, err = s.ensureAdminUser(c, dialInfo, "whomeverelse", "whateverelse")
	c.Assert(err, gc.ErrorMatches, `failed to add "whomeverelse" to admin database: cannot set admin password: not authorized .*`)
}

func (s *adminSuite) ensureAdminUser(c *gc.C, dialInfo *mgo.DialInfo, user, password string) (added bool, err error) {
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, gc.IsNil)
	port, err := strconv.Atoi(portString)
	c.Assert(err, gc.IsNil)
	return mongo.EnsureAdminUser(mongo.EnsureAdminUserParams{
		DialInfo: dialInfo,
		Port:     port,
		User:     user,
		Password: password,
	})
}

func (s *adminSuite) setUpMongo(c *gc.C) *mgo.DialInfo {
	inst := &gitjujutesting.MgoInstance{}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, gc.IsNil)
	s.AddCleanup(func(*gc.C) { inst.Destroy() })
	dialInfo := inst.DialInfo()
	dialInfo.Direct = true
	return dialInfo
}

func checkRoles(c *gc.C, session *mgo.Session, db, user string, expected []interface{}) {
	admin := session.DB("admin")

	var info map[string]interface{}
	err := admin.C("system.users").Find(bson.D{{"user", user}}).One(&info)
	c.Assert(err, gc.IsNil)

	var roles []interface{}
	for _, role := range info["roles"].([]interface{}) {
		switch role := role.(type) {
		case map[string]interface{}:
			// Mongo 2.6
			if role["db"] == db {
				roles = append(roles, role["role"])
			}
		default:
			// Mongo 2.4
			roles = append(roles, role)
		}
	}
	c.Assert(roles, jc.SameContents, expected)
}

func (s *adminSuite) TestSetAdminMongoPassword(c *gc.C) {
	dialInfo := s.setUpMongo(c)
	session, err := mgo.DialWithInfo(dialInfo)
	c.Assert(err, gc.IsNil)
	defer session.Close()

	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err = mongo.SetAdminMongoPassword(session, "auser", "")
	c.Assert(err, gc.IsNil)

	admin := session.DB("admin")
	err = mongo.SetAdminMongoPassword(session, "auser", "foo")
	c.Assert(err, gc.IsNil)
	err = admin.Login("auser", "")
	c.Assert(err, gc.ErrorMatches, "auth fail(s|ed)")
	err = admin.Login("auser", "foo")
	c.Assert(err, gc.IsNil)

	checkRoles(c, session, "admin", "auser",
		[]interface{}{
			string(mgo.RoleReadWriteAny),
			string(mgo.RoleDBAdminAny),
			string(mgo.RoleUserAdminAny),
			string(mgo.RoleClusterAdmin)})
}

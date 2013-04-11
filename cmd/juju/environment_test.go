package main

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"strings"
)

type GetEnvironmentSuite struct {
	repoSuite
}

var _ = Suite(&GetEnvironmentSuite{})

var singleValueTests = []struct {
	key    string
	output string
	err    string
}{
	{
		key:    "type",
		output: "dummy",
	}, {
		key:    "name",
		output: "dummyenv",
	}, {
		key:    "authorized-keys",
		output: "i-am-a-key",
	}, {
		key: "unknown",
		err: `Key "unknown" not found in "dummyenv" environment.`,
	},
}

func (s *GetEnvironmentSuite) TestSingleValue(c *C) {

	for _, t := range singleValueTests {
		context, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{t.key})
		if t.err != "" {
			c.Assert(err, ErrorMatches, t.err)
		} else {
			output := strings.TrimSpace(testing.Stdout(context))
			c.Assert(err, IsNil)
			c.Assert(output, Equals, t.output)
		}
	}
}

func (s *GetEnvironmentSuite) TestTooManyArgs(c *C) {
	_, err := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{"name", "type"})
	c.Assert(err, ErrorMatches, `unrecognized args: \["type"\]`)
}

func (s *GetEnvironmentSuite) TestAllValues(c *C) {

	context, _ := testing.RunCommand(c, &GetEnvironmentCommand{}, []string{})
	output := strings.TrimSpace(testing.Stdout(context))

	// Make sure that all the environment keys are there.
	any := "(.|\n)*" // because . doesn't match new lines.
	for key, _ := range s.Conn.Environ.Config().AllAttrs() {
		c.Assert(output, Matches, fmt.Sprintf("%s%s: %s", any, key, any))
	}
}
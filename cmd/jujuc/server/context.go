// The cmd/jujuc/server package implements the server side of the jujuc proxy
// tool, which forwards command invocations to the unit agent process so that
// they can be executed against specific state.
package server

import (
	"fmt"
	"launchpad.net/juju/go/cmd"
	"launchpad.net/juju/go/state"
	"os"
	"os/exec"
	"path/filepath"
)

// ClientContext is responsible for the state against which a jujuc-forwarded
// command will execute; it implements the core of the various jujuc tools, and
// is involved in constructing a suitable environment in which to execute a hook
// (which is likely to call jujuc tools that need this specific ClientContext).
type ClientContext struct {
	Id             string
	State          *state.State
	LocalUnitName  string
	RemoteUnitName string
	RelationName   string
}

// checkUnitState returns an error if ctx has nil State or LocalUnitName fields.
func (ctx *ClientContext) check() error {
	if ctx.State == nil {
		return fmt.Errorf("context %s cannot access state", ctx.Id)
	}
	if ctx.LocalUnitName == "" {
		return fmt.Errorf("context %s is not attached to a unit", ctx.Id)
	}
	return nil
}

// newCommands maps Command names to initializers.
var newCommands = map[string]func(*ClientContext) (cmd.Command, error){
	"close-port": NewClosePortCommand,
	"config-get": NewConfigGetCommand,
	"juju-log":   NewJujuLogCommand,
	"open-port":  NewOpenPortCommand,
	"unit-get":   NewUnitGetCommand,
}

// NewCommand returns an instance of the named Command, initialized to execute
// against this ClientContext.
func (ctx *ClientContext) NewCommand(name string) (cmd.Command, error) {
	f := newCommands[name]
	if f == nil {
		return nil, fmt.Errorf("unknown command: %s", name)
	}
	return f(ctx)
}

// hookVars returns an os.Environ-style list of strings necessary to run a hook
// such that it can know what environment it's operating in, and can call back
// into ctx.
func (ctx *ClientContext) hookVars(charmDir, socketPath string) []string {
	vars := []string{
		"APT_LISTCHANGES_FRONTEND=none",
		"DEBIAN_FRONTEND=noninteractive",
		"PATH=" + os.Getenv("PATH"),
		"CHARM_DIR=" + charmDir,
		"JUJU_CONTEXT_ID=" + ctx.Id,
		"JUJU_AGENT_SOCKET=" + socketPath,
	}
	if ctx.LocalUnitName != "" {
		vars = append(vars, "JUJU_UNIT_NAME="+ctx.LocalUnitName)
	}
	if ctx.RemoteUnitName != "" {
		vars = append(vars, "JUJU_REMOTE_UNIT="+ctx.RemoteUnitName)
	}
	if ctx.RelationName != "" {
		vars = append(vars, "JUJU_RELATION="+ctx.RelationName)
	}
	return vars
}

// RunHook executes a hook in an environment which allows it to to call back
// into ctx to execute jujuc tools.
func (ctx *ClientContext) RunHook(hookName, charmDir, socketPath string) error {
	ps := exec.Command(filepath.Join(charmDir, "hooks", hookName))
	ps.Env = ctx.hookVars(charmDir, socketPath)
	ps.Dir = charmDir
	if err := ps.Run(); err != nil {
		if ee, ok := err.(*exec.Error); ok {
			if os.IsNotExist(ee.Err) {
				return nil
			}
		}
		return err
	}
	return nil
}
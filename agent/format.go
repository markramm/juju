// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/jameinel/juju/utils"
)

// The format file in the agent config directory is used to identify the
// method of serialization.  This is used by individual format readers and
// writers to be able to translate from the file format to the in-memory
// structure.
//
// Juju only supports upgrading from single steps, so Juju only needs to know
// about the current format and the format of the previous stable release.
// For convenience, the format name includes the version number of the stable
// release that it will be released with.  Once this release has happened, the
// format should be considered FIXED, and should no longer be modified.  If
// changes are necessary to the format, a new format should be created.
//
// We don't need to create new formats for each release, the version number is
// just a convenience for us to know which stable release introduced that
// format.

const (
	formatFilename = "format"
	currentFormat  = format_1_16
	previousFormat = format_1_12
)

var (
	currentFormatter  = &formatter_1_16{}
	previousFormatter = &formatter_1_12{}
)

// The formatter defines the two methods needed by the formatters for
// translating to and from the internal, format agnostic, structure.
type formatter interface {
	read(dirName string) (*configInternal, error)
	write(config *configInternal) error
	writeCommands(config *configInternal) ([]string, error)
	// Migrate is called when upgrading from the previous format to the current format.
	migrate(config *configInternal)
}

func formatFile(dirName string) string {
	return path.Join(dirName, formatFilename)
}

func readFormat(dirName string) (string, error) {
	contents, err := ioutil.ReadFile(formatFile(dirName))
	// Once the previousFormat is defined to have a format file (1.16 or
	// above), not finding a format file should be a real error.
	if err != nil {
		return previousFormat, nil
	}
	return strings.TrimSpace(string(contents)), nil
}

func newFormatter(format string) (formatter, error) {
	switch format {
	case currentFormat:
		return currentFormatter, nil
	case previousFormat:
		return previousFormatter, nil
	}
	return nil, fmt.Errorf("unknown agent config format")
}

func writeFormatFile(dirName string, format string) error {
	if err := os.MkdirAll(dirName, 0755); err != nil {
		return err
	}
	newFile := path.Join(dirName, formatFilename+"-new")
	if err := ioutil.WriteFile(newFile, []byte(format+"\n"), 0644); err != nil {
		return err
	}
	return os.Rename(newFile, formatFile(dirName))
}

func writeFileCommands(filename, contents string, permission int) []string {
	quotedFilename := utils.ShQuote(filename)
	return []string{
		fmt.Sprintf("install -m %o /dev/null %s", permission, quotedFilename),
		fmt.Sprintf(`printf '%%s\n' %s > %s`, utils.ShQuote(contents), quotedFilename),
	}
}

func writeCommandsForFormat(dirName, format string) []string {
	commands := []string{"mkdir -p " + utils.ShQuote(dirName)}
	commands = append(commands, writeFileCommands(formatFile(dirName), format, 0644)...)
	return commands
}

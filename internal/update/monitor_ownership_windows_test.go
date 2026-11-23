//go:build windows

package update

import (
	"os/exec"
	"testing"
)

func startLegacyMonitorFixture(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	runWindowsFixture(t, cmd)
}

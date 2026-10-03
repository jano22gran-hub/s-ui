package dnstunnel

import (
	"os/exec"
	"syscall"
)

// bindToParent kills the tunnel when the panel dies, even by SIGKILL, so a
// restarted panel does not find port 53 still held by an orphan.
func bindToParent(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGKILL}
}

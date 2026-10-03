//go:build !linux

package dnstunnel

import "os/exec"

func bindToParent(cmd *exec.Cmd) {}

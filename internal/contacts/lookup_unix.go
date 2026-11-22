//go:build unix

package contacts

import (
	"os/exec"
	"syscall"
)

// setProcessGroup isola il figlio in un proprio gruppo di processi (Piano 9, review C).
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup uccide il gruppo: anche i discendenti muoiono con il comando.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

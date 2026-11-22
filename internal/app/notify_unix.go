//go:build unix

package app

import (
	"os/exec"
	"syscall"
)

// setNotifyProcessGroup mette il comando in un proprio gruppo di processi e, allo
// scadere del timeout, uccide l'intero gruppo (Piano 11, review punto 4): un figlio
// del comando non resta vivo dopo la notifica.
func setNotifyProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

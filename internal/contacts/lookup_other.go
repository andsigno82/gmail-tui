//go:build !unix

package contacts

import "os/exec"

// setProcessGroup: su piattaforme senza gruppi di processi unix il figlio è solo il processo diretto.
func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup uccide il solo processo diretto (Piano 9, review C).
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

//go:build !unix

package app

import "os/exec"

// setNotifyProcessGroup su Windows non crea un gruppo di processi: il timeout uccide
// solo il processo diretto (limite documentato nel README, Piano 11, review punto 4).
func setNotifyProcessGroup(cmd *exec.Cmd) {}

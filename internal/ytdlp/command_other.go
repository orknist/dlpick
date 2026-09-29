//go:build !unix

package ytdlp

import "os/exec"

func configureCommand(cmd *exec.Cmd) {}

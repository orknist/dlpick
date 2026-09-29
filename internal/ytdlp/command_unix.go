//go:build unix

package ytdlp

import (
	"os/exec"
	"syscall"
	"time"
)

// configureCommand puts yt-dlp in its own process group so Ctrl+C also
// reaches ffmpeg, which yt-dlp spawns for merges and audio conversion.
func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGINT)
	}
	cmd.WaitDelay = 3 * time.Second
}

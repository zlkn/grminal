//go:build linux || darwin

package pane

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// osPTY is the production PTY: a real pseudo-terminal running a child process.
// It satisfies the PTY seam, so panes are oblivious to whether they wrap this or
// the in-memory fake used in tests.
type osPTY struct {
	f   *os.File
	cmd *exec.Cmd
}

// StartShell launches the user's $SHELL (falling back to /bin/sh) attached to a
// new pseudo-terminal sized cols×rows, and returns it as a PTY.
func StartShell(cols, rows int) (PTY, error) {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		return nil, err
	}
	return &osPTY{f: f, cmd: cmd}, nil
}

func (p *osPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *osPTY) Write(b []byte) (int, error) { return p.f.Write(b) }

func (p *osPTY) Resize(rows, cols uint16) error {
	return pty.Setsize(p.f, &pty.Winsize{Rows: rows, Cols: cols})
}

// Close terminates the child and releases the pseudo-terminal.
func (p *osPTY) Close() error {
	err := p.f.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
		_, _ = p.cmd.Process.Wait()
	}
	return err
}

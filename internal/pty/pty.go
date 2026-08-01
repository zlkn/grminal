package pty

import "io"

// PTY is the seam between a Pane and the operating-system pseudo-terminal.
//
// It is deliberately narrow so tests can substitute an in-memory fake and the
// unit suite never touches a real PTY. The production implementation is a thin
// adapter over github.com/creack/pty, exercised only by a build-tagged
// integration test.
type PTY interface {
	io.ReadWriteCloser
	// Resize informs the PTY of a new window size, in character rows and cols.
	Resize(rows, cols uint16) error
}

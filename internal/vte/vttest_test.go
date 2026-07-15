package vte

import (
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/yzolkin/go-vte/internal/testutil"
)

// vttestSession drives the real vttest(1) binary through our Parser+Grid inside
// a headless PTY. It answers device queries via SetReply (wired back to the
// PTY), so vttest behaves as if it were talking to a live terminal — no GPU or
// display required. The resulting Grid.Dump is snapshotted as a golden file.
//
// The suite is opt-in: it needs the external `vttest` binary, spawns a child
// process, and relies on output-quiescence timing rather than a hard protocol
// barrier. Plain `go test ./...` therefore skips it; enable with VTTEST=1.
// Regenerate goldens with `VTTEST=1 go test ./internal/vte -run Vttest -update`
// and review the dump before committing.
type vttestSession struct {
	t       *testing.T
	f       *os.File
	cmd     *exec.Cmd
	g       *Grid
	lastAct atomic.Int64 // UnixNano of the last byte parsed
	mu      sync.Mutex   // guards Grid between the reader goroutine and dumps
}

// startVttest launches vttest in a cols×rows PTY and begins parsing its output.
func startVttest(t *testing.T, cols, rows int) *vttestSession {
	t.Helper()
	if os.Getenv("VTTEST") == "" {
		t.Skip("set VTTEST=1 to run the vttest integration suite")
	}
	bin, err := exec.LookPath("vttest")
	if err != nil {
		t.Skipf("vttest not found in PATH: %v", err)
	}

	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
	if err != nil {
		t.Fatalf("start vttest: %v", err)
	}

	g := NewGrid(cols, rows)
	p := NewParser(g)
	p.SetReply(f) // device-query answers are delivered to vttest's stdin

	s := &vttestSession{t: t, f: f, cmd: cmd, g: g}
	s.lastAct.Store(time.Now().UnixNano())

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.mu.Lock()
				p.Write(buf[:n])
				s.mu.Unlock()
				s.lastAct.Store(time.Now().UnixNano())
			}
			if err != nil {
				return // PTY closed on cleanup
			}
		}
	}()

	t.Cleanup(func() {
		_ = f.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return s
}

// send writes keystrokes to vttest's stdin.
func (s *vttestSession) send(keys string) {
	s.t.Helper()
	if _, err := s.f.WriteString(keys); err != nil {
		s.t.Fatalf("send %q: %v", keys, err)
	}
}

// waitQuiet blocks until no bytes have been parsed for `quiet`, or until
// `timeout` elapses. A timeout is a soft failure: the caller dumps anyway, but
// the log line flags that the screen may not have settled.
func (s *vttestSession) waitQuiet(quiet, timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if time.Since(time.Unix(0, s.lastAct.Load())) >= quiet {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Logf("waitQuiet: timed out after %s (output still active)", timeout)
}

// dump returns the current grid snapshot as text.
func (s *vttestSession) dump() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.g.Dump()
}

// step presses RETURN once and blocks until the screen actually changes (or a
// timeout). vttest gates every screen on RETURN and only redraws after reading
// the next keystroke, so syncing on an observed screen change — rather than a
// fixed delay — is what makes the advance deterministic instead of racy.
func (s *vttestSession) step() {
	s.t.Helper()
	before := s.dump()
	s.send("\r")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		s.waitQuiet(150*time.Millisecond, 500*time.Millisecond)
		if s.dump() != before {
			return
		}
	}
}

// advanceUntil steps with RETURN until the grid contains marker, stopping as
// soon as it appears so the snapshot lands on that exact screen. It fatals if
// marker never shows up within maxSteps.
func (s *vttestSession) advanceUntil(marker string, maxSteps int) {
	s.t.Helper()
	s.waitQuiet(300*time.Millisecond, 2*time.Second)
	for range maxSteps {
		if strings.Contains(s.dump(), marker) {
			return
		}
		s.step()
	}
	s.t.Fatalf("screen marker %q not reached after %d steps; last screen:\n%s",
		marker, maxSteps, s.dump())
}

// TestVttestCursorMovements records/compares the first screen of vttest's
// "Test of cursor movements" (menu option 1) against a golden snapshot.
func TestVttestCursorMovements(t *testing.T) {
	s := startVttest(t, 80, 24)
	// Let vttest draw its menu and finish probing the terminal (DA/DSR/size).
	s.waitQuiet(300*time.Millisecond, 5*time.Second)
	if got := s.dump(); !strings.Contains(got, "Test of cursor movements") {
		t.Fatalf("vttest menu did not appear; got:\n%s", got)
	}
	// Select test 1 and advance to its first bordered screen — the classic
	// "*'s and +'s around the edge" alignment page.
	s.send("1\r")
	s.advanceUntil("unbroken", 8)
	testutil.Golden(t, "vttest_cursor_1", []byte(s.dump()))
}

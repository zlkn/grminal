package parser_test

import (
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/yzolkin/go-vte/internal/testutil"
	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

func TestVttest(t *testing.T) {
	if os.Getenv("VTTEST") == "" {
		t.Skip("VTTEST environment variable not set; skipping integration test")
	}

	vttestPath, err := exec.LookPath("vttest")
	if err != nil {
		t.Skip("vttest binary not found in PATH; skipping integration test")
	}

	cmd := exec.Command(vttestPath)
	cmd.Env = append(os.Environ(), "TERM=vt100")
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		t.Fatalf("failed to start vttest under pty: %v", err)
	}
	defer ptmx.Close()

	g := vte.NewGrid(80, 24)
	p := parser.NewParser(g)
	p.SetReply(ptmx)

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				p.Write(buf[:n])
			}
			if err != nil {
				if err == io.EOF {
					done <- nil
				} else {
					done <- err
				}
				return
			}
		}
	}()

	send := func(s string) {
		time.Sleep(50 * time.Millisecond)
		_, _ = io.WriteString(ptmx, s)
	}

	send("1\n")
	send("\n")
	send("q\n")
	send("q\n")

	select {
	case err := <-done:
		if err != nil {
			t.Logf("pty drain finished with: %v", err)
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("vttest run timed out after 3s")
	}

	testutil.Golden(t, "vttest_menu", []byte(g.Dump()))
}

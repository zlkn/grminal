package pty_test

import (
	"sync"
	"testing"
	"time"

	"github.com/yzolkin/go-vte/internal/pty"
)

func TestConcurrentSnapshotAndRun(t *testing.T) {
	fake := &fakePTY{}
	// Pre-fill read buffer with repetitive ANSI data.
	for i := 0; i < 1000; i++ {
		fake.readBuf.WriteString("line of text for testing\r\n")
	}

	p := pty.NewPane(fake, 80, 24, 1000)

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer/Drain thread
	go func() {
		defer wg.Done()
		_ = p.Run()
	}()

	// Reader thread (mimicking Ebiten render loop)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			_ = p.Snapshot()
			_ = p.SnapshotScrolled(i % 10)
			time.Sleep(1 * time.Millisecond)
		}
	}()

	wg.Wait()
}

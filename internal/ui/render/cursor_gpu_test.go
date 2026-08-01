package render

import (
	"image/color"
	"os"
	"os/exec"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/vte"
)

// gpuChildEnv marks the re-executed child process that owns the Ebiten loop.
const gpuChildEnv = "GO_VTE_GPU_CHILD"

// Pixel-level check of what the cursor actually paints — the only test covering
// drawCursor itself rather than its pure helpers. Ebiten can only read pixels back
// from inside a running game loop, so this drives one real frame and reads the
// result there. That needs a display, hence the opt-in env var (same convention as
// the vttest suite).
//
// It runs the frame in a re-executed child process because ebiten.RunGame can only
// be called once per process and tears the graphics/font context down on return:
// in-process, every later test that shapes text would fail. The child does the
// work; the parent just reports its output.
//
//	GPUTEST=1 go test ./internal/render -run CursorPixels -v
func TestCursorPixels(t *testing.T) {
	if os.Getenv("GPUTEST") == "" {
		t.Skip("set GPUTEST=1 to run the headless-GPU cursor test (needs a display)")
	}
	if os.Getenv(gpuChildEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), gpuChildEnv+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("cursor pixel frame failed in the child process: %v\n%s", err, out)
		}
		return
	}

	cases := []cursorPixelCase{
		{
			name:    "block fills the cell",
			style:   "block",
			focused: true,
			painted: []probe{{0.5, 0.5}},
			clear:   []probe{{1.5, 0.5}, {0.5, 1.5}}, // neighbouring cells untouched
		},
		{
			name:    "underline sits on the bottom edge",
			style:   "underline",
			focused: true,
			painted: []probe{{0.5, 0.97}},
			clear:   []probe{{0.5, 0.4}},
		},
		{
			name:    "beam sits on the left edge",
			style:   "beam",
			focused: true,
			painted: []probe{{0.02, 0.5}},
			clear:   []probe{{0.6, 0.5}},
		},
		{
			name:    "unfocused draws a hollow outline",
			style:   "block",
			focused: false,
			painted: []probe{{0.5, 0.99}, {0.01, 0.5}}, // edges
			clear:   []probe{{0.5, 0.5}},               // hollow middle
		},
		{
			name:    "blinked off paints nothing",
			style:   "block",
			focused: true,
			blink:   true,
			blinkOn: false,
			clear:   []probe{{0.5, 0.5}},
		},
		{
			name:    "blinked on paints the cursor",
			style:   "block",
			focused: true,
			blink:   true,
			blinkOn: true,
			painted: []probe{{0.5, 0.5}},
		},
		{
			name:    "hidden cursor paints nothing",
			style:   "block",
			focused: true,
			hidden:  true,
			clear:   []probe{{0.5, 0.5}},
		},
		{
			name:    "opacity 1 paints the exact cursor colour",
			style:   "block",
			focused: true,
			opacity: 1,
			painted: []probe{{0.5, 0.5}},
			opaque:  true,
		},
		{
			name:    "DECSCUSR overrides the configured shape",
			style:   "block", // config says block...
			shape:   vte.CursorShapeBar,
			focused: true,
			painted: []probe{{0.02, 0.5}}, // ...the app's bar wins
			clear:   []probe{{0.6, 0.5}},
		},
	}

	g := &cursorPixelGame{t: t, cases: cases}
	ebiten.SetWindowSize(320, 240)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatalf("RunGame: %v", err)
	}
	if !g.ran {
		t.Fatal("the frame never rendered; no pixels were checked")
	}
}

// probe is a point inside the cursor cell in cell-relative fractions (0..1).
type probe struct{ fx, fy float64 }

type cursorPixelCase struct {
	name            string
	style           string          // configured cursor_style
	shape           vte.CursorShape // DECSCUSR override (Default = none)
	focused, blink  bool
	blinkOn, hidden bool
	opacity         float64 // 0 means "use the default"
	opaque          bool    // assert the painted pixel equals the cursor colour exactly
	painted, clear  []probe
}

// cursorPixelGame renders every case in a single frame, reads the pixels back and
// reports failures, then terminates.
type cursorPixelGame struct {
	t     *testing.T
	cases []cursorPixelCase
	ran   bool
}

func (g *cursorPixelGame) Update() error {
	if g.ran {
		return ebiten.Termination
	}
	return nil
}

func (g *cursorPixelGame) Layout(w, h int) (int, int) { return w, h }

func (g *cursorPixelGame) Draw(_ *ebiten.Image) {
	if g.ran {
		return
	}
	g.ran = true

	for _, tc := range g.cases {
		cfg := config.Default()
		cfg.CursorStyle = tc.style
		cfg.CursorBlink = tc.blink
		if tc.opacity > 0 {
			cfg.CursorOpacity = tc.opacity
		}

		r, err := NewRenderer(cfg, 1)
		if err != nil {
			g.t.Errorf("%s: NewRenderer: %v", tc.name, err)
			continue
		}
		cellW, cellH := r.CellSize()
		dst := ebiten.NewImage(int(cellW*4), int(r.TabBarHeight()+cellH*4))

		snap := blankSnapshot(4, 4)
		snap.CursorVisible = !tc.hidden
		snap.CursorShape = tc.shape
		r.Draw(dst, snap, CursorState{Focused: tc.focused, BlinkOn: tc.blinkOn})

		// Cursor cell (0,0) starts below the reserved tab bar; padding defaults to 0.
		originY := r.TabBarHeight()
		at := func(p probe) color.Color {
			return dst.At(int(p.fx*cellW), int(originY+p.fy*cellH))
		}
		bg := color.RGBA(cfg.Background)

		for _, p := range tc.painted {
			got := at(p)
			if sameColor(got, bg) {
				g.t.Errorf("%s: pixel at cell fraction (%.2f,%.2f) = background %v, want the cursor drawn",
					tc.name, p.fx, p.fy, got)
			}
			if tc.opaque && !sameColor(got, color.RGBA(cfg.Cursor)) {
				g.t.Errorf("%s: pixel at (%.2f,%.2f) = %v, want the opaque cursor colour %v",
					tc.name, p.fx, p.fy, got, cfg.Cursor)
			}
		}
		for _, p := range tc.clear {
			if got := at(p); !sameColor(got, bg) {
				g.t.Errorf("%s: pixel at cell fraction (%.2f,%.2f) = %v, want the untouched background %v",
					tc.name, p.fx, p.fy, got, bg)
			}
		}
		dst.Deallocate()
	}
}

// blankSnapshot is a cols×rows snapshot of empty cells with the cursor at (0,0),
// so the only thing the renderer can paint over the background is the cursor.
func blankSnapshot(cols, rows int) vte.Snapshot {
	cells := make([]vte.Cell, cols*rows)
	for i := range cells {
		cells[i] = vte.Cell{Rune: ' '}
	}
	return vte.Snapshot{Cols: cols, Rows: rows, Cells: cells, CursorVisible: true}
}

// sameColor compares two colors in their premultiplied 16-bit form, tolerating the
// rounding a GPU blend introduces.
func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	const tol = 0x0200 // ~2/255
	return near(ar, br, tol) && near(ag, bg, tol) && near(ab, bb, tol) && near(aa, ba, tol)
}

func near(a, b, tol uint32) bool {
	if a > b {
		return a-b <= tol
	}
	return b-a <= tol
}

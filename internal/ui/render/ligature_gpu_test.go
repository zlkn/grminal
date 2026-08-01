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

// The payoff of font_snap, measured in pixels: the same ligature must rasterize
// identically wherever it sits inside a run.
//
// Without a whole-pixel advance, a glyph's sub-pixel phase comes from its offset
// within the style run (column × the font's fractional advance), and text/v2 bakes
// that phase into the bitmap it caches. So `!=` five cells into a line was drawn
// from a different, softer bitmap than `!=` at the start of it — 4 distinct rasters
// across 8 offsets at 16px. A whole-pixel advance collapses that to 1.
//
// Runs the frame in a child process for the reason described on TestCursorPixels.
//
//	GPUTEST=1 go test ./internal/render -run LigatureRaster -v
func TestLigatureRasterUniformity(t *testing.T) {
	if os.Getenv("GPUTEST") == "" {
		t.Skip("set GPUTEST=1 to run the headless-GPU ligature test (needs a display)")
	}
	if os.Getenv(gpuChildEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), gpuChildEnv+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("ligature raster frame failed in the child process: %v\n%s", err, out)
		}
		return
	}

	g := &ligRasterGame{t: t}
	ebiten.SetWindowSize(320, 240)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatalf("RunGame: %v", err)
	}
	if !g.ran {
		t.Fatal("the frame never rendered; no rasters were compared")
	}
}

type ligRasterGame struct {
	t   *testing.T
	ran bool
}

func (g *ligRasterGame) Update() error {
	if g.ran {
		return ebiten.Termination
	}
	return nil
}

func (g *ligRasterGame) Layout(w, h int) (int, int) { return w, h }

func (g *ligRasterGame) Draw(_ *ebiten.Image) {
	if g.ran {
		return
	}
	g.ran = true

	for _, lig := range []string{"!=", "->", "=>"} {
		for _, snap := range []bool{true, false} {
			n := g.distinctRasters(lig, snap)
			switch {
			case snap && n != 1:
				g.t.Errorf("font_snap on: %q takes %d distinct rasters across 8 offsets in a run, want 1",
					lig, n)
			case !snap && n == 1:
				// Not a failure in itself — it means this font/size has no drift, so the
				// snapped case proves nothing. Flag it so the test is not silently vacuous.
				g.t.Errorf("font_snap off: %q is already uniform, so this test cannot detect the defect it guards",
					lig)
			}
		}
	}
}

// distinctRasters renders lig at eight offsets inside one style run and reports how
// many distinct ink patterns come back. A leading 'x' anchors the run (so the blanks
// after it are interior and survive SplitRuns' trimming) and the blanks put the
// ligature at a varying run-relative offset with no neighbouring ink to confuse the
// crop.
func (g *ligRasterGame) distinctRasters(lig string, snap bool) int {
	cfg := config.Default()
	cfg.FontSize, cfg.FontSnap = 16, snap
	r, err := NewRenderer(cfg, 1)
	if err != nil {
		g.t.Fatal(err)
	}
	cellW, cellH := r.CellSize()
	bg := color.RGBA(cfg.Background)
	runes := []rune(lig)

	seen := map[string]bool{}
	for off := 0; off < 8; off++ {
		cols := off + len(runes) + 4
		cells := make([]vte.Cell, cols)
		for i := range cells {
			cells[i] = vte.Cell{Rune: ' '}
		}
		cells[0] = vte.Cell{Rune: 'x'} // run anchor
		for i, ru := range runes {
			cells[off+2+i] = vte.Cell{Rune: ru}
		}

		img := ebiten.NewImage(int(cellW*float64(cols)), int(r.TabBarHeight()+cellH))
		r.Draw(img, vte.Snapshot{Cols: cols, Rows: 1, Cells: cells}, CursorState{})

		// Crop the ligature's ink, skipping the anchor cell.
		minX, minY, maxX, maxY := img.Bounds().Dx(), img.Bounds().Dy(), -1, -1
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := int(cellW * 2); x < img.Bounds().Dx(); x++ {
				if sameColor(img.At(x, y), bg) {
					continue
				}
				minX, minY = min(minX, x), min(minY, y)
				maxX, maxY = max(maxX, x), max(maxY, y)
			}
		}
		if maxX < 0 {
			g.t.Fatalf("%q at offset %d: nothing was drawn", lig, off)
		}
		key := ""
		for y := minY; y <= maxY; y++ {
			for x := minX; x <= maxX; x++ {
				cr, cg, cb, _ := img.At(x, y).RGBA()
				key += string(rune('0' + (cr+cg+cb)/3*9/0xffff)) // 10-level coverage
			}
			key += "|"
		}
		seen[key] = true
		img.Deallocate()
	}
	return len(seen)
}

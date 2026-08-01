package render

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/vte"
)

// Pixel-level checks of the coverage-gamma correction. Ebiten can only read
// pixels back from inside a running game loop, so these drive one real frame in
// a child process — same convention and reason as TestCursorPixels.
//
//	GPUTEST=1 go test ./internal/ui/render -run TextGamma -v
func TestTextGamma(t *testing.T) {
	if os.Getenv("GPUTEST") == "" {
		t.Skip("set GPUTEST=1 to run the headless-GPU text gamma test (needs a display)")
	}
	if os.Getenv(gpuChildEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), gpuChildEnv+"=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("text gamma frame failed in the child process: %v\n%s", err, out)
		}
		return
	}

	g := &gammaGame{t: t}
	ebiten.SetWindowSize(320, 240)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatalf("RunGame: %v", err)
	}
	if !g.ran {
		t.Fatal("the frame never rendered; nothing was measured")
	}
}

type gammaGame struct {
	t   *testing.T
	ran bool
}

func (g *gammaGame) Update() error {
	if g.ran {
		return ebiten.Termination
	}
	return nil
}

func (g *gammaGame) Layout(w, h int) (int, int) { return w, h }

func (g *gammaGame) Draw(_ *ebiten.Image) {
	if g.ran {
		return
	}
	g.ran = true

	g.identity()
	g.darkens()
	g.faintRatioFalls()
}

// identity is the safety net for the whole change: with the exponent neutral,
// the shader path must reproduce the DrawImage path it replaced. Anything else
// means the shader is altering colour, premultiplication or sampling on its own,
// and every other result here would be measuring that bug instead of the gamma.
func (g *gammaGame) identity() {
	plain := renderLig(g.t, 1)

	shaded := newGammaRenderer(g.t, 1.4)
	if shaded.textShader == nil {
		g.t.Fatal("gamma 1.4 did not compile a shader; nothing to compare")
	}
	shaded.invGamma = 1 // keep the shader, neutralise only the exponent
	shaded2 := drawLig(g.t, shaded)

	if len(plain) != len(shaded2) {
		g.t.Fatalf("raster sizes differ: %d vs %d pixels", len(plain), len(shaded2))
	}
	// One step of 8-bit quantisation: the shader does the same multiply in
	// floats, so only rounding may differ.
	const tol = 1.0 / 255
	worst, at := 0.0, -1
	for i := range plain {
		if d := abs(plain[i] - shaded2[i]); d > worst {
			worst, at = d, i
		}
	}
	if worst > tol {
		g.t.Errorf("shader at exponent 1 differs from the plain path by %.4f at pixel %d (tolerance %.4f)",
			worst, at, tol)
	}
}

// darkens: the point of the feature. Every partially covered pixel must gain
// ink, none may lose it, and the average must actually move — a correction that
// only shifts a handful of pixels is not worth a shader.
func (g *gammaGame) darkens() {
	off := renderLig(g.t, 1)
	on := renderLig(g.t, 1.4)

	var sumOff, sumOn float64
	lighter := 0
	for i := range off {
		sumOff += off[i]
		sumOn += on[i]
		if on[i] < off[i]-1.0/255 {
			lighter++
		}
	}
	if lighter > 0 {
		g.t.Errorf("%d pixels lost coverage at gamma 1.4; the curve must be monotonic", lighter)
	}
	meanOff, meanOn := sumOff/float64(len(off)), sumOn/float64(len(on))
	if meanOn <= meanOff {
		g.t.Errorf("mean coverage %.4f at gamma 1.4, want more than %.4f at gamma 1", meanOn, meanOff)
	}
	g.t.Logf("mean coverage over inked pixels: %.4f -> %.4f", mean(inkPixels(off)), mean(inkPixels(on)))
}

// faintRatioFalls pins the effect gamma can actually be held to: fewer barely-
// there pixels. Those faint ones are the haze around a stroke, and a monotonic
// curve promotes them into it.
//
// Note what is deliberately *not* asserted here. The share of pixels in the
// mushy middle (0.15..0.85) barely moves, because a monotonic curve slides the
// whole distribution up rather than emptying a band around its centre. That
// share is the metric the blur was originally diagnosed with, and gamma does not
// fix it — the three-row spread of a ligature bar is geometry, and only
// grid-fitting moves ink between rows. It is logged, not asserted, so the
// distinction stays on the record instead of being quietly rounded away.
func (g *gammaGame) faintRatioFalls() {
	off, on := renderLig(g.t, 1), renderLig(g.t, 1.4)

	faintOff, faintOn := ratioBelow(off, 0.5), ratioBelow(on, 0.5)
	if faintOn >= faintOff {
		g.t.Errorf("faint-pixel ratio %.3f at gamma 1.4, want below %.3f at gamma 1", faintOn, faintOff)
	}
	g.t.Logf("faint-pixel ratio (<0.5): %.3f -> %.3f", faintOff, faintOn)
	g.t.Logf("soft-pixel ratio (0.15..0.85, not expected to move): %.3f -> %.3f",
		ratioBetween(off, 0.15, 0.85), ratioBetween(on, 0.15, 0.85))
}

// inkPixels drops background pixels, leaving the ones a glyph actually touched.
func inkPixels(cov []float64) []float64 {
	ink := make([]float64, 0, len(cov))
	for _, c := range cov {
		if c >= 0.02 {
			ink = append(ink, c)
		}
	}
	return ink
}

func ratioBelow(cov []float64, hi float64) float64 {
	return ratioBetween(cov, 0, hi)
}

// ratioBetween is the share of inked pixels whose coverage lies in [lo, hi].
func ratioBetween(cov []float64, lo, hi float64) float64 {
	ink := inkPixels(cov)
	if len(ink) == 0 {
		return 0
	}
	n := 0
	for _, c := range ink {
		if c >= lo && c <= hi {
			n++
		}
	}
	return float64(n) / float64(len(ink))
}

// newGammaRenderer builds a renderer for the default theme at the given gamma.
func newGammaRenderer(t *testing.T, gamma float64) *Renderer {
	t.Helper()
	cfg := config.Default()
	cfg.FontSize, cfg.TextGamma = 16, gamma
	r, err := NewRenderer(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func renderLig(t *testing.T, gamma float64) []float64 {
	t.Helper()
	return drawLig(t, newGammaRenderer(t, gamma))
}

// drawLig renders "!=" through r and returns each pixel's ink coverage, recovered
// from the composited frame by undoing the fg-over-bg blend. Working in coverage
// rather than raw colour keeps the assertions readable and theme-independent.
func drawLig(t *testing.T, r *Renderer) []float64 {
	t.Helper()
	const lig = "!="
	runes := []rune(lig)
	cells := make([]vte.Cell, len(runes))
	for i, ru := range runes {
		cells[i] = vte.Cell{Rune: ru}
	}
	cellW, cellH := r.CellSize()
	w := int(cellW*float64(len(cells))) + int(r.padL) + int(r.padR)
	h := int(r.TabBarHeight()+cellH) + int(r.padT) + int(r.padB)

	img := ebiten.NewImage(w, h)
	defer img.Deallocate()
	r.Draw(img, vte.Snapshot{Cols: len(cells), Rows: 1, Cells: cells}, CursorState{})

	bg := lum(r.defaultBG)
	fg := lum(r.defaultFG)
	span := bg - fg
	if span == 0 {
		t.Fatal("theme foreground and background have equal luminance; coverage is unrecoverable")
	}
	cov := make([]float64, 0, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := clampF((bg-lum(toRGBA(img.At(x, y))))/span, 0, 1)
			cov = append(cov, c)
		}
	}
	return cov
}

// FONTDUMP writes a visual comparison strip instead of asserting anything: the
// same ligature line at a range of gammas, stacked, for picking a default by eye
// on the display that will actually show it. Numbers alone cannot settle that.
//
//	FONTDUMP=1 GPUTEST=1 go test ./internal/ui/render -run FontDump -v
func TestFontDump(t *testing.T) {
	if os.Getenv("FONTDUMP") == "" {
		t.Skip("set FONTDUMP=1 (with a display) to write /tmp/govte_fontdump.png")
	}
	if os.Getenv(gpuChildEnv) == "" {
		cmd := exec.Command(os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), gpuChildEnv+"=1")
		out, err := cmd.CombinedOutput()
		t.Log(string(out))
		if err != nil {
			t.Fatalf("font dump frame failed in the child process: %v", err)
		}
		return
	}
	g := &dumpGame{t: t}
	ebiten.SetWindowSize(320, 240)
	if err := ebiten.RunGame(g); err != nil {
		t.Fatalf("RunGame: %v", err)
	}
}

type dumpGame struct {
	t   *testing.T
	ran bool
}

func (g *dumpGame) Update() error {
	if g.ran {
		return ebiten.Termination
	}
	return nil
}

func (g *dumpGame) Layout(w, h int) (int, int) { return w, h }

func (g *dumpGame) Draw(_ *ebiten.Image) {
	if g.ran {
		return
	}
	g.ran = true

	const line = "a != b -> c => d >= e === f |> g"
	gammas := []float64{1.0, 1.2, 1.4, 1.8}
	scale := ebiten.Monitor().DeviceScaleFactor()

	var strips []*image.RGBA
	maxW := 0
	for _, gamma := range gammas {
		cfg := config.Default()
		cfg.TextGamma = gamma
		cfg.PaddingLeft, cfg.PaddingRight, cfg.PaddingTop, cfg.PaddingBottom = 0, 0, 0, 0
		r, err := NewRenderer(cfg, scale)
		if err != nil {
			g.t.Fatal(err)
		}
		cellW, cellH := r.CellSize()
		runes := []rune(line)
		cells := make([]vte.Cell, len(runes))
		for i, ru := range runes {
			cells[i] = vte.Cell{Rune: ru}
		}
		w, h := int(cellW*float64(len(cells))), int(r.TabBarHeight()+cellH)
		img := ebiten.NewImage(w, h)
		r.Draw(img, vte.Snapshot{Cols: len(cells), Rows: 1, Cells: cells}, CursorState{})

		top := int(r.TabBarHeight())
		strip := image.NewRGBA(image.Rect(0, 0, w, h-top))
		for y := top; y < h; y++ {
			for x := 0; x < w; x++ {
				strip.Set(x, y-top, toRGBA(img.At(x, y)))
			}
		}
		img.Deallocate()
		strips = append(strips, strip)
		if w > maxW {
			maxW = w
		}
		g.t.Logf("gamma %.1f: cell %.0fx%.0f", gamma, cellW, cellH)
	}

	totalH := 0
	for _, s := range strips {
		totalH += s.Bounds().Dy()
	}
	out := image.NewRGBA(image.Rect(0, 0, maxW, totalH))
	y := 0
	for _, s := range strips {
		b := s.Bounds()
		for yy := 0; yy < b.Dy(); yy++ {
			for xx := 0; xx < b.Dx(); xx++ {
				out.Set(xx, y+yy, s.At(xx, yy))
			}
		}
		y += b.Dy()
	}

	const path = "/tmp/govte_fontdump.png"
	f, err := os.Create(path)
	if err != nil {
		g.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, out); err != nil {
		g.t.Fatal(err)
	}
	fmt.Printf("wrote %s (%dx%d), rows top to bottom: %v\n", path, maxW, totalH, gammas)
}

func lum(c color.RGBA) float64 {
	return 0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)
}

func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

func mean(vs []float64) float64 {
	if len(vs) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vs {
		sum += v
	}
	return sum / float64(len(vs))
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

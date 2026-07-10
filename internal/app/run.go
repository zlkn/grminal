package app

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/pane"
	"github.com/yzolkin/go-vte/internal/render"
)

const (
	initialWidth  = 900
	initialHeight = 600
)

// game is the Ebitengine adapter. It reconciles one pane (shell) per tab, routes
// input, and draws the active tab. Splits within a tab are not wired yet — the
// layout package is ready, but multiple live PTYs per tab is a follow-up.
type game struct {
	app   *App
	cfg   config.Config
	r     *render.Renderer
	panes map[int]*pane.Pane // keyed by tab ID
	scale float64            // current device scale factor
	cols  int
	rows  int
}

// Run starts the application: it opens a window and runs the terminal until the
// window is closed.
func Run() error {
	cfg, _ := config.Load(config.Path()) // config.Load logs read/parse issues itself

	g := &game{
		app:   New(),
		cfg:   cfg,
		panes: make(map[int]*pane.Pane),
	}
	if err := g.setScale(1); err != nil { // real scale is applied in LayoutF
		return err
	}

	ebiten.SetWindowSize(initialWidth, initialHeight)
	ebiten.SetWindowTitle("go-vte")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	return ebiten.RunGame(g)
}

// setScale (re)builds the renderer for a device scale factor so glyphs are
// rasterized at native resolution instead of being upscaled (blurry).
func (g *game) setScale(s float64) error {
	if s <= 0 {
		s = 1
	}
	r, err := render.NewRenderer(g.cfg, s)
	if err != nil {
		return err
	}
	g.r = r
	g.scale = s
	return nil
}

// Update advances one frame: reconcile panes with tabs, then handle input.
func (g *game) Update() error {
	g.reconcilePanes()
	g.handleInput()
	return nil
}

// Draw paints the active tab's pane full-window.
func (g *game) Draw(screen *ebiten.Image) {
	if p := g.active(); p != nil {
		g.r.Draw(screen, p.Snapshot())
	}
}

// Layout is required by ebiten.Game but superseded by LayoutF below.
func (g *game) Layout(_, _ int) (int, int) { return 1, 1 }

// LayoutF renders at the monitor's device scale (physical pixels) so text is
// crisp on HiDPI displays, and reflows panes when the cell grid changes.
func (g *game) LayoutF(outsideW, outsideH float64) (float64, float64) {
	if s := ebiten.Monitor().DeviceScaleFactor(); s > 0 && s != g.scale {
		_ = g.setScale(s)
	}
	pw, ph := outsideW*g.scale, outsideH*g.scale

	cols, rows := g.r.GridSize(int(pw), int(ph))
	if cols != g.cols || rows != g.rows {
		g.cols, g.rows = cols, rows
		for _, p := range g.panes {
			_ = p.Resize(cols, rows)
		}
	}
	return pw, ph
}

// active returns the pane backing the active tab, or nil if it has none yet.
func (g *game) active() *pane.Pane { return g.panes[g.app.Active().ID] }

// reconcilePanes spawns a shell for any tab lacking a pane and tears down panes
// whose tab has been closed.
func (g *game) reconcilePanes() {
	for _, tb := range g.app.Tabs() {
		if _, ok := g.panes[tb.ID]; ok {
			continue
		}
		pt, err := pane.StartShell(g.cols, g.rows)
		if err != nil {
			continue
		}
		p := pane.NewPane(pt, g.cols, g.rows, g.cfg.ScrollbackLines)
		g.panes[tb.ID] = p
		go func() { _ = p.Run() }()
	}

	live := make(map[int]bool, len(g.app.Tabs()))
	for _, tb := range g.app.Tabs() {
		live[tb.ID] = true
	}
	for id, p := range g.panes {
		if !live[id] {
			_ = p.Close()
			delete(g.panes, id)
		}
	}
}

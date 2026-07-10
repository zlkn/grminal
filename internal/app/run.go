package app

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/pane"
	"github.com/yzolkin/go-vte/internal/render"
)

const (
	fontSize      = 16
	initialWidth  = 900
	initialHeight = 600
)

// game is the Ebitengine adapter. It reconciles one pane (shell) per tab, routes
// input, and draws the active tab. Splits within a tab are not wired yet — the
// layout package is ready, but multiple live PTYs per tab is a follow-up.
type game struct {
	app   *App
	r     *render.Renderer
	panes map[int]*pane.Pane // keyed by tab ID
	cols  int
	rows  int
}

// Run starts the application: it opens a window and runs the terminal until the
// window is closed.
func Run() error {
	r, err := render.NewRenderer(fontSize)
	if err != nil {
		return err
	}
	cols, rows := r.GridSize(initialWidth, initialHeight)

	g := &game{
		app:   New(),
		r:     r,
		panes: make(map[int]*pane.Pane),
		cols:  cols,
		rows:  rows,
	}

	ebiten.SetWindowSize(initialWidth, initialHeight)
	ebiten.SetWindowTitle("go-vte")
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	return ebiten.RunGame(g)
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

// Layout maps the window size to the logical screen and reflows panes when the
// cell grid dimensions change.
func (g *game) Layout(w, h int) (int, int) {
	cols, rows := g.r.GridSize(w, h)
	if cols != g.cols || rows != g.rows {
		g.cols, g.rows = cols, rows
		for _, p := range g.panes {
			_ = p.Resize(cols, rows)
		}
	}
	return w, h
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
		p := pane.NewPane(pt, g.cols, g.rows)
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

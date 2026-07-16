package app

import (
	"strconv"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/pane"
	"github.com/yzolkin/go-vte/internal/render"
	"github.com/yzolkin/go-vte/internal/vte"
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

	// Mouse-reporting state carried across frames: the cell the pointer last sat
	// in (to detect motion) and the button currently held (for drag reports).
	mouseCol, mouseRow int
	mouseBtn           vte.MouseButton
	mouseHeld          bool

	// Key autorepeat state and its timing in ticks (derived from config ms at
	// startup). Ebiten reports only press edges, so we re-emit held keys here.
	repeat                      keyRepeat
	repeatDelay, repeatInterval int

	// dirty marks that the terminal changed and the offscreen frame must be
	// re-rendered. It is set by any source of visible change (PTY output, input,
	// resize, tab reconcile) and cleared when the frame is rebuilt. Written from
	// the PTY goroutine and the Ebiten loop, hence atomic.
	dirty atomic.Bool
	// frame is a persistent offscreen buffer holding the fully-rendered terminal.
	// We re-render into it only when dirty, but blit it to the screen every frame
	// (double buffering). This keeps the display correct despite Ebiten's
	// multi-buffered presentation — a skipped screen write can otherwise present a
	// stale back buffer, causing ghosting (duplicated rows) in apps like neovim.
	frame *ebiten.Image
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
	g.app.SetKeys(cfg.Keys)
	g.repeatDelay = msToTicks(cfg.KeyRepeatDelayMs)
	g.repeatInterval = msToTicks(cfg.KeyRepeatIntervalMs)
	if g.repeatInterval < 1 {
		g.repeatInterval = 1
	}
	if err := g.setScale(1); err != nil { // real scale is applied in LayoutF
		return err
	}

	ebiten.SetWindowSize(initialWidth, initialHeight)
	ebiten.SetWindowTitle("go-vte")
	ebiten.SetWindowDecorated(cfg.WindowDecorated) // false = borderless (no title bar)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	// Demand-driven rendering: the expensive frame render runs only when the
	// terminal changed; each frame just blits the persistent offscreen buffer.
	// The screen need not auto-clear since that blit fully covers it.
	ebiten.SetScreenClearedEveryFrame(false)
	g.dirty.Store(true) // paint the first frame
	return ebiten.RunGame(g)
}

// msToTicks converts a millisecond duration to Ebiten update ticks at the
// current TPS. A non-positive input yields 0 (which disables autorepeat).
func msToTicks(ms int) int {
	if ms <= 0 {
		return 0
	}
	return ms * ebiten.TPS() / 1000
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

// Update advances one frame: reconcile panes with tabs, close any whose shell
// has exited (quitting when the last one does), then handle input.
func (g *game) Update() error {
	g.reconcilePanes()
	if g.closeExitedPanes() {
		return ebiten.Termination
	}
	g.handleInput()
	return nil
}

// closeExitedPanes closes the tab of a pane whose shell has exited (e.g. after
// Ctrl+D). It returns true when the last tab's shell has exited, signalling the
// caller to quit. It acts on at most one tab per call; reconcilePanes tears down
// the pane on the next tick.
func (g *game) closeExitedPanes() (quit bool) {
	for _, tb := range g.app.Tabs() {
		p := g.panes[tb.ID]
		if p == nil || !p.Exited() {
			continue
		}
		if g.app.Count() == 1 {
			return true // last shell exited → close the window
		}
		g.app.CloseTab(tb.ID)
		g.dirty.Store(true)
		break
	}
	return false
}

// Draw blits the offscreen frame to the screen, re-rendering that frame first
// only when something changed. Blitting every frame (rather than skipping the
// screen write) keeps the display correct under Ebiten's multi-buffered
// presentation; the expensive render work is what the dirty flag gates.
func (g *game) Draw(screen *ebiten.Image) {
	g.ensureFrame(screen)
	if g.dirty.Swap(false) {
		g.renderFrame()
	}
	screen.DrawImage(g.frame, nil)
}

// ensureFrame (re)allocates the offscreen buffer to match the screen size,
// forcing a re-render when the size changes.
func (g *game) ensureFrame(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if g.frame != nil && g.frame.Bounds().Dx() == w && g.frame.Bounds().Dy() == h {
		return
	}
	if g.frame != nil {
		g.frame.Deallocate()
	}
	g.frame = ebiten.NewImage(w, h)
	g.dirty.Store(true) // the fresh buffer starts blank and must be painted
}

// renderFrame paints the active pane and the tab bar into the offscreen frame.
func (g *game) renderFrame() {
	if p := g.active(); p != nil {
		g.r.Draw(g.frame, p.Snapshot())
	}
	g.r.DrawTabBar(g.frame, g.tabLabels(), g.app.ActiveIndex())
}

// tabLabels builds each tab's bar label from the live per-tab OSC titles.
func (g *game) tabLabels() []string {
	tabs := g.app.Tabs()
	titles := make([]string, len(tabs))
	for i, tb := range tabs {
		if p := g.panes[tb.ID]; p != nil {
			titles[i] = p.Title()
		}
	}
	return formatTabLabels(titles)
}

// formatTabLabels renders each tab's bar label as "<index> <title>" (index only
// when the title is empty). Pure, so it is unit-tested.
func formatTabLabels(titles []string) []string {
	labels := make([]string, len(titles))
	for i, title := range titles {
		label := strconv.Itoa(i + 1)
		if title != "" {
			label += " " + title
		}
		labels[i] = label
	}
	return labels
}

// Layout is required by ebiten.Game but superseded by LayoutF below.
func (g *game) Layout(_, _ int) (int, int) { return 1, 1 }

// LayoutF renders at the monitor's device scale (physical pixels) so text is
// crisp on HiDPI displays, and reflows panes when the cell grid changes.
func (g *game) LayoutF(outsideW, outsideH float64) (float64, float64) {
	if s := ebiten.Monitor().DeviceScaleFactor(); s > 0 && s != g.scale {
		_ = g.setScale(s)
		g.dirty.Store(true)
	}
	pw, ph := outsideW*g.scale, outsideH*g.scale

	cols, rows := g.r.GridSize(int(pw), int(ph))
	if cols != g.cols || rows != g.rows {
		g.cols, g.rows = cols, rows
		for _, p := range g.panes {
			_ = p.Resize(cols, rows)
		}
		g.dirty.Store(true)
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
		p.SetChangeHook(func() { g.dirty.Store(true) }) // redraw on PTY output
		g.panes[tb.ID] = p
		g.dirty.Store(true)
		go func() {
			_ = p.Run()
			g.dirty.Store(true) // wake the loop so Update closes the dead tab
		}()
	}

	live := make(map[int]bool, len(g.app.Tabs()))
	for _, tb := range g.app.Tabs() {
		live[tb.ID] = true
	}
	for id, p := range g.panes {
		if !live[id] {
			_ = p.Close()
			delete(g.panes, id)
			g.dirty.Store(true)
		}
	}
}

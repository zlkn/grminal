package app

import (
	"strconv"
	"sync/atomic"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/pty"
	"github.com/yzolkin/go-vte/internal/ui/render"
	"github.com/yzolkin/go-vte/internal/vte"
)

const (
	initialWidth  = 900
	initialHeight = 600
)

// game is the Ebitengine adapter. It reconciles one pane (shell) per tab, routes
// input, and draws the active tab.
type game struct {
	app   *App
	cfg   config.Config
	r     *render.Renderer
	panes map[int]*pty.Pane // keyed by tab ID
	scale float64           // current device scale factor
	cols  int
	rows  int

	mouseCol, mouseRow int
	mouseBtn           vte.MouseButton
	mouseHeld          bool

	repeat                      keyRepeat
	repeatDelay, repeatInterval int

	altHeld     int
	altNumDelay int
	showTabNums bool

	scrollOff   int
	scrollAccum float64

	blinkTick   int
	blinkPeriod int
	blinkOn     bool
	blinking    bool
	focused     bool

	dirty atomic.Bool
	frame *ebiten.Image
}

// Run starts the application: it opens a window and runs the terminal until the
// window is closed.
func Run() error {
	cfg, _ := config.Load(config.Path())

	g := &game{
		app:   New(),
		cfg:   cfg,
		panes: make(map[int]*pty.Pane),
	}
	g.app.SetKeys(cfg.Keys)
	g.repeatDelay = msToTicks(cfg.KeyRepeatDelayMs)
	g.repeatInterval = msToTicks(cfg.KeyRepeatIntervalMs)
	if g.repeatInterval < 1 {
		g.repeatInterval = 1
	}
	g.altNumDelay = msToTicks(500)
	g.blinkPeriod = msToTicks(cursorBlinkMs)
	g.blinkOn, g.focused = true, true

	if err := g.setScale(1); err != nil {
		return err
	}
	// Compute initial grid dimensions from default window size so shells start at proper size
	g.cols, g.rows = g.r.GridSize(initialWidth, initialHeight)

	ebiten.SetWindowSize(initialWidth, initialHeight)
	ebiten.SetWindowTitle("go-vte")
	ebiten.SetWindowDecorated(cfg.WindowDecorated)
	ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
	ebiten.SetScreenClearedEveryFrame(false)
	g.dirty.Store(true)
	return ebiten.RunGame(g)
}

func msToTicks(ms int) int {
	if ms <= 0 {
		return 0
	}
	return ms * ebiten.TPS() / 1000
}

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

func (g *game) Update() error {
	g.reconcilePanes()
	if g.closeExitedPanes() {
		return ebiten.Termination
	}
	g.updateTabNumbers()
	g.updateCursorBlink()
	g.handleInput()
	return nil
}

const cursorBlinkMs = 530

func (g *game) updateCursorBlink() {
	if focused := ebiten.IsFocused(); focused != g.focused {
		g.focused = focused
		g.resetBlink()
	}
	p := g.active()
	blinking := p != nil && g.focused && g.scrollOff == 0 &&
		p.CursorVisible() && g.r.CursorBlinking(p.CursorBlink())
	if blinking != g.blinking {
		g.blinking = blinking
		g.resetBlink()
	}
	if !blinking {
		return
	}
	g.blinkTick++
	if on := blinkPhase(g.blinkTick, g.blinkPeriod); on != g.blinkOn {
		g.blinkOn = on
		g.dirty.Store(true)
	}
}

func blinkPhase(tick, period int) bool {
	if period <= 0 {
		return true
	}
	return (tick/period)%2 == 0
}

func (g *game) resetBlink() {
	g.blinkTick, g.blinkOn = 0, true
	g.dirty.Store(true)
}

func (g *game) closeExitedPanes() (quit bool) {
	for _, tb := range g.app.Tabs() {
		p := g.panes[tb.ID]
		if p == nil || !p.Exited() {
			continue
		}
		if g.app.Count() == 1 {
			return true
		}
		g.app.CloseTab(tb.ID)
		g.dirty.Store(true)
		break
	}
	return false
}

func (g *game) Draw(screen *ebiten.Image) {
	g.ensureFrame(screen)
	if g.dirty.Swap(false) {
		g.renderFrame()
	}
	screen.DrawImage(g.frame, nil)
}

func (g *game) ensureFrame(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	if g.frame != nil && g.frame.Bounds().Dx() == w && g.frame.Bounds().Dy() == h {
		return
	}
	if g.frame != nil {
		g.frame.Deallocate()
	}
	g.frame = ebiten.NewImage(w, h)
	g.dirty.Store(true)
}

func (g *game) renderFrame() {
	if p := g.active(); p != nil {
		cur := render.CursorState{Focused: g.focused, BlinkOn: g.blinkOn}
		g.r.Draw(g.frame, p.SnapshotScrolled(g.scrollOff), cur)
	}
	g.r.DrawTabBar(g.frame, g.tabLabels(), g.app.ActiveIndex())
}

func (g *game) updateTabNumbers() {
	if ebiten.IsKeyPressed(ebiten.KeyAlt) {
		g.altHeld++
	} else {
		g.altHeld = 0
	}
	show := g.altNumDelay > 0 && g.altHeld >= g.altNumDelay
	if show != g.showTabNums {
		g.showTabNums = show
		g.dirty.Store(true)
	}
}

func (g *game) tabLabels() []string {
	tabs := g.app.Tabs()
	titles := make([]string, len(tabs))
	for i, tb := range tabs {
		if p := g.panes[tb.ID]; p != nil {
			titles[i] = p.Title()
		}
	}
	return formatTabLabels(titles, g.showTabNums)
}

func formatTabLabels(titles []string, showNumbers bool) []string {
	labels := make([]string, len(titles))
	for i, title := range titles {
		num := strconv.Itoa(i + 1)
		switch {
		case title == "":
			labels[i] = num
		case showNumbers:
			labels[i] = num + " " + title
		default:
			labels[i] = title
		}
	}
	return labels
}

func (g *game) Layout(_, _ int) (int, int) { return 1, 1 }

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

func (g *game) active() *pty.Pane { return g.panes[g.app.Active().ID] }

func (g *game) reconcilePanes() {
	for _, tb := range g.app.Tabs() {
		if _, ok := g.panes[tb.ID]; ok {
			continue
		}
		pt, err := pty.StartShell(g.cols, g.rows)
		if err != nil {
			continue
		}
		p := pty.NewPane(pt, g.cols, g.rows, g.cfg.ScrollbackLines)
		p.SetChangeHook(func() { g.dirty.Store(true) })
		g.panes[tb.ID] = p
		g.dirty.Store(true)
		go func() {
			_ = p.Run()
			g.dirty.Store(true)
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

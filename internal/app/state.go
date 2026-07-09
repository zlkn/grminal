package app

import "unicode"

// Tab is a single terminal tab. It will own a LayoutManager and its panes; for
// now it carries just an identity so the tab-management logic can be tested in
// isolation.
type Tab struct {
	ID    int
	Title string
	// TODO(stage-3/4): Layout *layout.Layout and its panes.
}

// App is the top-level state (AppState): it owns the ordered set of tabs, tracks
// the active one, and translates hotkeys into tab actions. It is a pure state
// machine — no GPU, no OS input — so it is fully unit-testable.
type App struct {
	tabs   []*Tab
	active int
	nextID int
}

// New returns an app with a single active tab.
func New() *App {
	a := &App{}
	a.NewTab()
	a.active = 0
	return a
}

// Count returns the number of tabs.
func (a *App) Count() int { return len(a.tabs) }

// ActiveIndex returns the index of the active tab.
func (a *App) ActiveIndex() int { return a.active }

// Active returns the active tab.
func (a *App) Active() *Tab { return a.tabs[a.active] }

// Tabs returns the tabs in order.
func (a *App) Tabs() []*Tab { return a.tabs }

// NewTab appends a fresh tab and makes it active.
func (a *App) NewTab() *Tab {
	tb := &Tab{ID: a.nextID}
	a.nextID++
	a.tabs = append(a.tabs, tb)
	a.active = len(a.tabs) - 1
	return tb
}

// CloseTab removes the tab with the given id, adjusting the active index. It
// refuses to close the last remaining tab or an unknown id (returns false).
func (a *App) CloseTab(id int) bool {
	if len(a.tabs) <= 1 {
		return false
	}
	idx := -1
	for i, tb := range a.tabs {
		if tb.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return false
	}

	a.tabs = append(a.tabs[:idx], a.tabs[idx+1:]...)
	switch {
	case a.active > idx:
		a.active--
	case a.active == idx && a.active >= len(a.tabs):
		a.active = len(a.tabs) - 1
	}
	return true
}

// Next activates the next tab, wrapping around.
func (a *App) Next() { a.active = (a.active + 1) % len(a.tabs) }

// Prev activates the previous tab, wrapping around.
func (a *App) Prev() { a.active = (a.active - 1 + len(a.tabs)) % len(a.tabs) }

// Switch activates the tab at index i, returning false if i is out of range.
func (a *App) Switch(i int) bool {
	if i < 0 || i >= len(a.tabs) {
		return false
	}
	a.active = i
	return true
}

// Key is an injected keyboard event, decoupled from any input backend so the
// hotkey logic is testable.
type Key struct {
	Rune             rune
	Ctrl, Shift, Alt bool
}

// HandleKey applies a global hotkey and reports whether it was consumed. Keys
// that are not hotkeys return false so the caller can forward them to the
// focused pane.
func (a *App) HandleKey(k Key) bool {
	r := unicode.ToLower(k.Rune)
	switch {
	case k.Ctrl && k.Shift && r == 't':
		a.NewTab()
		return true
	case k.Ctrl && k.Shift && r == 'w':
		a.CloseTab(a.Active().ID)
		return true
	case k.Ctrl && k.Shift && r == ']':
		a.Next()
		return true
	case k.Ctrl && k.Shift && r == '[':
		a.Prev()
		return true
	case k.Alt && r >= '1' && r <= '9':
		a.Switch(int(r - '1'))
		return true
	}
	return false
}

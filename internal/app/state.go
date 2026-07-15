package app

import "github.com/yzolkin/go-vte/internal/config"

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
	keys   config.Keybindings
}

// New returns an app with a single active tab and the built-in keybindings.
func New() *App {
	a := &App{keys: config.DefaultKeys()}
	a.NewTab()
	a.active = 0
	return a
}

// SetKeys replaces the tab-management hotkeys (e.g. from the user config).
func (a *App) SetKeys(k config.Keybindings) { a.keys = k }

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
// hotkey logic is testable. Name is the normalized key token (lowercase single
// char like "t"/"]"/"1", or a named key like "tab").
type Key struct {
	Name             string
	Ctrl, Shift, Alt bool
}

// HandleKey applies a global hotkey and reports whether it was consumed. Keys
// that are not hotkeys return false so the caller can forward them to the
// focused pane. Tab actions come from the configurable bindings.
func (a *App) HandleKey(k Key) bool {
	switch {
	case matches(a.keys.NewTab, k):
		a.NewTab()
		return true
	case matches(a.keys.CloseTab, k):
		a.CloseTab(a.Active().ID)
		return true
	case matches(a.keys.NextTab, k):
		a.Next()
		return true
	case matches(a.keys.PrevTab, k):
		a.Prev()
		return true
	}
	return false
}

// matches reports whether key k satisfies binding b. An empty binding (no key)
// matches nothing.
func matches(b config.Binding, k Key) bool {
	return b.Key != "" &&
		b.Key == k.Name &&
		b.Ctrl == k.Ctrl && b.Shift == k.Shift && b.Alt == k.Alt
}

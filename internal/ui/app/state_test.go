package app

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/config"
)

func TestNewAppHasOneActiveTab(t *testing.T) {
	a := New()
	if a.Count() != 1 {
		t.Fatalf("Count = %d, want 1", a.Count())
	}
	if a.ActiveIndex() != 0 {
		t.Errorf("ActiveIndex = %d, want 0", a.ActiveIndex())
	}
	if a.Active() == nil {
		t.Error("Active() = nil")
	}
}

func TestNewTabSwitchesToIt(t *testing.T) {
	a := New()
	tb := a.NewTab()
	if a.Count() != 2 {
		t.Fatalf("Count = %d, want 2", a.Count())
	}
	if a.ActiveIndex() != 1 || a.Active() != tb {
		t.Errorf("active = %d/%p, want 1/%p", a.ActiveIndex(), a.Active(), tb)
	}
}

func TestCloseActiveTab(t *testing.T) {
	a := New()
	first := a.Active()
	second := a.NewTab()

	if !a.CloseTab(second.ID) {
		t.Fatal("CloseTab returned false")
	}
	if a.Count() != 1 {
		t.Fatalf("Count = %d, want 1", a.Count())
	}
	if a.Active() != first || a.ActiveIndex() != 0 {
		t.Errorf("active = %p/%d, want %p/0", a.Active(), a.ActiveIndex(), first)
	}
}

func TestCloseNonActiveAdjustsIndex(t *testing.T) {
	a := New()
	_ = a.Active()  // idx 0
	mid := a.NewTab() // idx 1
	last := a.NewTab() // idx 2, active

	a.CloseTab(mid.ID) // removing an index below active shifts active down
	if a.Count() != 2 {
		t.Fatalf("Count = %d, want 2", a.Count())
	}
	if a.ActiveIndex() != 1 || a.Active() != last {
		t.Errorf("active = %d/%p, want 1/%p", a.ActiveIndex(), a.Active(), last)
	}
}

func TestCannotCloseLastTab(t *testing.T) {
	a := New()
	if a.CloseTab(a.Active().ID) {
		t.Error("closing the only tab should fail")
	}
	if a.Count() != 1 {
		t.Errorf("Count = %d, want 1", a.Count())
	}
}

func TestNextPrevWrap(t *testing.T) {
	a := New()
	a.NewTab()
	a.NewTab() // 3 tabs, active idx 2

	a.Next()
	if a.ActiveIndex() != 0 {
		t.Errorf("after Next from last, active = %d, want 0 (wrap)", a.ActiveIndex())
	}
	a.Prev()
	if a.ActiveIndex() != 2 {
		t.Errorf("after Prev from first, active = %d, want 2 (wrap)", a.ActiveIndex())
	}
}

func TestSwitch(t *testing.T) {
	a := New()
	a.NewTab()
	a.NewTab()

	if !a.Switch(0) || a.ActiveIndex() != 0 {
		t.Errorf("Switch(0) failed: active = %d", a.ActiveIndex())
	}
	if a.Switch(9) {
		t.Error("Switch(9) should fail on out-of-range index")
	}
}

// --- hotkeys via injected key events -----------------------------------------

func TestHotkeyNewTab(t *testing.T) {
	a := New()
	if !a.HandleKey(Key{Name: "t", Ctrl: true, Shift: true}) {
		t.Fatal("Ctrl+Shift+T not consumed")
	}
	if a.Count() != 2 {
		t.Errorf("Count = %d, want 2", a.Count())
	}
}

func TestHotkeyCloseTab(t *testing.T) {
	a := New()
	a.NewTab()
	if !a.HandleKey(Key{Name: "w", Ctrl: true, Shift: true}) {
		t.Fatal("Ctrl+Shift+W not consumed")
	}
	if a.Count() != 1 {
		t.Errorf("Count = %d, want 1", a.Count())
	}
}

func TestHotkeyNextTabCtrlTab(t *testing.T) {
	a := New()
	a.NewTab() // 2 tabs, active idx 1
	if !a.HandleKey(Key{Name: "tab", Ctrl: true}) {
		t.Fatal("Ctrl+Tab not consumed")
	}
	if a.ActiveIndex() != 0 {
		t.Errorf("ActiveIndex = %d, want 0 (wrapped)", a.ActiveIndex())
	}
}

func TestHotkeyPrevTabCtrlShiftTab(t *testing.T) {
	a := New()
	a.NewTab() // 2 tabs, active idx 1
	if !a.HandleKey(Key{Name: "tab", Ctrl: true, Shift: true}) {
		t.Fatal("Ctrl+Shift+Tab not consumed")
	}
	if a.ActiveIndex() != 0 {
		t.Errorf("ActiveIndex = %d, want 0", a.ActiveIndex())
	}
}

func TestCustomKeybindingViaSetKeys(t *testing.T) {
	a := New()
	a.SetKeys(config.Keybindings{NextTab: config.Binding{Key: "l", Alt: true}})
	a.NewTab() // 2 tabs, active idx 1
	// Old default (Ctrl+Tab) is gone; the custom Alt+L drives Next.
	if a.HandleKey(Key{Name: "tab", Ctrl: true}) {
		t.Error("Ctrl+Tab should no longer be bound")
	}
	if !a.HandleKey(Key{Name: "l", Alt: true}) {
		t.Fatal("Alt+L not consumed")
	}
	if a.ActiveIndex() != 0 {
		t.Errorf("ActiveIndex = %d, want 0 (wrapped)", a.ActiveIndex())
	}
}

func TestNonHotkeyNotConsumed(t *testing.T) {
	a := New()
	if a.HandleKey(Key{Name: "a"}) {
		t.Error("plain 'a' should not be consumed as a hotkey")
	}
	// Plain Tab (no modifiers) is not a hotkey — it must reach the pane.
	if a.HandleKey(Key{Name: "tab"}) {
		t.Error("plain Tab should not be consumed as a hotkey")
	}
}

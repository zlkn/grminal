package config

import (
	"bytes"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// captureLog redirects the standard logger to a buffer for the duration of fn.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	fn()
	return buf.String()
}

func TestLoadLogsMissingWhenDebug(t *testing.T) {
	old := Debug
	Debug = true
	defer func() { Debug = old }()

	path := filepath.Join(t.TempDir(), "missing.toml")
	out := captureLog(t, func() { _, _ = Load(path) })
	if !strings.Contains(out, "no config file") || !strings.Contains(out, path) {
		t.Errorf("debug log = %q, want mention of missing file and path", out)
	}
}

func TestLoadSilentWhenNotDebug(t *testing.T) {
	old := Debug
	Debug = false
	defer func() { Debug = old }()

	path := filepath.Join(t.TempDir(), "missing.toml")
	if out := captureLog(t, func() { _, _ = Load(path) }); out != "" {
		t.Errorf("expected no log for missing file without debug, got %q", out)
	}
}

func TestLoadLogsParseErrorAlways(t *testing.T) {
	old := Debug
	Debug = false // errors are logged even without debug
	defer func() { Debug = old }()

	path := filepath.Join(t.TempDir(), "c.toml")
	if err := os.WriteFile(path, []byte("font_size = nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureLog(t, func() { _, _ = Load(path) })
	if !strings.Contains(out, "config") {
		t.Errorf("expected parse-error log, got %q", out)
	}
}

func TestParseHexColor(t *testing.T) {
	tests := []struct {
		in   string
		want color.RGBA
		ok   bool
	}{
		{"#424242", color.RGBA{0x42, 0x42, 0x42, 0xff}, true},
		{"ffffff", color.RGBA{0xff, 0xff, 0xff, 0xff}, true},
		{"#20bbfc", color.RGBA{0x20, 0xbb, 0xfc, 0xff}, true},
		{"#12345", color.RGBA{}, false},  // too short
		{"#zzzzzz", color.RGBA{}, false}, // not hex
		{"", color.RGBA{}, false},
	}
	for _, tc := range tests {
		got, err := parseHexColor(tc.in)
		if tc.ok && err != nil {
			t.Errorf("parseHexColor(%q) unexpected error: %v", tc.in, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("parseHexColor(%q) expected error, got %v", tc.in, got)
		}
		if tc.ok && got != tc.want {
			t.Errorf("parseHexColor(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDefault(t *testing.T) {
	c := Default()
	if c.FontSize != 16 {
		t.Errorf("FontSize = %v, want 16", c.FontSize)
	}
	if c.Foreground != (color.RGBA{0x42, 0x42, 0x42, 0xff}) {
		t.Errorf("Foreground = %v, want #424242", c.Foreground)
	}
	if c.Background != (color.RGBA{0xf0, 0xee, 0xe6, 0xff}) {
		t.Errorf("Background = %v, want #f0eee6", c.Background)
	}
	if c.Palette[1] != (color.RGBA{0xb8, 0x1a, 0x6b, 0xff}) {
		t.Errorf("Palette[1] = %v, want #b81a6b", c.Palette[1])
	}
	if c.ScrollbackLines != 10000 {
		t.Errorf("ScrollbackLines = %d, want 10000", c.ScrollbackLines)
	}
	if c.IconFillRatio != 0.85 {
		t.Errorf("IconFillRatio = %v, want 0.85", c.IconFillRatio)
	}
	if c.CursorStyle != "block" {
		t.Errorf("CursorStyle = %q, want block", c.CursorStyle)
	}
	if !c.FontSnap {
		t.Error("FontSnap = false, want true by default")
	}
	if c.CursorOpacity != 0.6 {
		t.Errorf("CursorOpacity = %v, want 0.6", c.CursorOpacity)
	}
	if c.CursorBlink {
		t.Error("CursorBlink = true, want false by default")
	}
	if c.PaddingTop != 0 || c.PaddingRight != 0 || c.PaddingBottom != 0 || c.PaddingLeft != 0 {
		t.Errorf("padding = %d/%d/%d/%d, want all 0",
			c.PaddingTop, c.PaddingRight, c.PaddingBottom, c.PaddingLeft)
	}
	if !c.WindowDecorated {
		t.Error("WindowDecorated = false, want true by default")
	}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load(missing) error = %v, want nil", err)
	}
	if c != Default() {
		t.Errorf("Load(missing) = %+v, want Default()", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	content := `
# go-vte config
font_size = 18
background = "#101010"
color1 = "#ff0000"
scrollback_lines = 5000
cursor_style = "beam"
font_snap = false
cursor_opacity = 0.35
cursor_blink = true
icon_fill_ratio = 0.9
padding_left = 12
padding_top = 8
padding_right = 4
padding_bottom = 6
window_decorated = false
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.FontSize != 18 {
		t.Errorf("FontSize = %v, want 18", c.FontSize)
	}
	if c.Background != (color.RGBA{0x10, 0x10, 0x10, 0xff}) {
		t.Errorf("Background = %v, want #101010", c.Background)
	}
	if c.Palette[1] != (color.RGBA{0xff, 0x00, 0x00, 0xff}) {
		t.Errorf("Palette[1] = %v, want #ff0000", c.Palette[1])
	}
	if c.ScrollbackLines != 5000 {
		t.Errorf("ScrollbackLines = %d, want 5000", c.ScrollbackLines)
	}
	if c.CursorStyle != "beam" {
		t.Errorf("CursorStyle = %q, want beam", c.CursorStyle)
	}
	if c.FontSnap {
		t.Error("FontSnap = true, want false from the file")
	}
	if c.CursorOpacity != 0.35 {
		t.Errorf("CursorOpacity = %v, want 0.35", c.CursorOpacity)
	}
	if !c.CursorBlink {
		t.Error("CursorBlink = false, want true")
	}
	if c.IconFillRatio != 0.9 {
		t.Errorf("IconFillRatio = %v, want 0.9", c.IconFillRatio)
	}
	if c.PaddingLeft != 12 || c.PaddingTop != 8 || c.PaddingRight != 4 || c.PaddingBottom != 6 {
		t.Errorf("padding = L%d T%d R%d B%d, want 12/8/4/6",
			c.PaddingLeft, c.PaddingTop, c.PaddingRight, c.PaddingBottom)
	}
	if c.WindowDecorated {
		t.Error("WindowDecorated = true, want false from config")
	}
	// Untouched keys keep defaults.
	if c.Foreground != Default().Foreground {
		t.Errorf("Foreground changed unexpectedly: %v", c.Foreground)
	}
}

func TestParseBinding(t *testing.T) {
	ok := []struct {
		in   string
		want Binding
	}{
		{"ctrl+tab", Binding{Key: "tab", Ctrl: true}},
		{"ctrl+shift+tab", Binding{Key: "tab", Ctrl: true, Shift: true}},
		{"CTRL+Shift+T", Binding{Key: "t", Ctrl: true, Shift: true}},
		{"alt+]", Binding{Key: "]", Alt: true}},
		{"control+w", Binding{Key: "w", Ctrl: true}},
		{"1", Binding{Key: "1"}},
	}
	for _, tc := range ok {
		got, err := parseBinding(tc.in)
		if err != nil {
			t.Errorf("parseBinding(%q) error: %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("parseBinding(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}

	bad := []string{"", "ctrl+", "meta+t", "ctrl+esc", "+"}
	for _, in := range bad {
		if _, err := parseBinding(in); err == nil {
			t.Errorf("parseBinding(%q): expected error", in)
		}
	}
}

func TestLoadKeybindingOverride(t *testing.T) {
	content := `
key_next_tab = "ctrl+tab"
key_prev_tab = "ctrl+shift+tab"
key_new_tab  = "alt+n"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if c.Keys.NextTab != (Binding{Key: "tab", Ctrl: true}) {
		t.Errorf("NextTab = %+v", c.Keys.NextTab)
	}
	if c.Keys.NewTab != (Binding{Key: "n", Alt: true}) {
		t.Errorf("NewTab = %+v", c.Keys.NewTab)
	}
	// Unspecified binding keeps its default.
	if c.Keys.CloseTab != DefaultKeys().CloseTab {
		t.Errorf("CloseTab = %+v, want default", c.Keys.CloseTab)
	}
}

func TestLoadMalformedFallsBack(t *testing.T) {
	content := `
font_size = notanumber
color2 = "#00ff00"
bogusline
color99 = "#123456"
`
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := Load(path)
	if err == nil {
		t.Error("Load(malformed) expected error, got nil")
	}
	// Bad font_size keeps default; valid color2 still applied.
	if c.FontSize != Default().FontSize {
		t.Errorf("FontSize = %v, want default %v on bad value", c.FontSize, Default().FontSize)
	}
	if c.Palette[2] != (color.RGBA{0x00, 0xff, 0x00, 0xff}) {
		t.Errorf("Palette[2] = %v, want #00ff00 (valid line applied)", c.Palette[2])
	}
}

// An opacity outside [0,1] is rejected rather than clamped, so "60" meaning 60%
// is reported instead of silently drawing an opaque cursor over the glyph.
func TestLoadRejectsOutOfRangeOpacity(t *testing.T) {
	for _, val := range []string{"60", "-0.5", "1.5", "half"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("cursor_opacity = "+val+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := Load(path)
		if err == nil {
			t.Errorf("cursor_opacity = %s: expected an error, got nil", val)
		}
		if c.CursorOpacity != Default().CursorOpacity {
			t.Errorf("cursor_opacity = %s: kept %v, want the default %v",
				val, c.CursorOpacity, Default().CursorOpacity)
		}
	}
}

// text_gamma is an exponent applied to every glyph pixel, so a typo is far more
// likely than an intent: values outside [0.5,3] are reported and the default is
// kept rather than rendering text that has washed out or smeared into blobs.
func TestLoadTextGamma(t *testing.T) {
	t.Run("accepted", func(t *testing.T) {
		for _, val := range []string{"1", "1.4", "0.5", "3"} {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("text_gamma = "+val+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			c, err := Load(path)
			if err != nil {
				t.Errorf("text_gamma = %s: unexpected error %v", val, err)
			}
			if want, _ := strconv.ParseFloat(val, 64); c.TextGamma != want {
				t.Errorf("text_gamma = %s: got %v, want %v", val, c.TextGamma, want)
			}
		}
	})

	t.Run("rejected", func(t *testing.T) {
		for _, val := range []string{"0", "-1", "0.2", "10", "none"} {
			path := filepath.Join(t.TempDir(), "config.toml")
			if err := os.WriteFile(path, []byte("text_gamma = "+val+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			c, err := Load(path)
			if err == nil {
				t.Errorf("text_gamma = %s: expected an error, got nil", val)
			}
			if c.TextGamma != Default().TextGamma {
				t.Errorf("text_gamma = %s: kept %v, want the default %v",
					val, c.TextGamma, Default().TextGamma)
			}
		}
	})
}

func TestPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/xdg")
	if got, want := Path(), filepath.Join("/tmp/xdg", "go-vte", "config.toml"); got != want {
		t.Errorf("Path() with XDG = %q, want %q", got, want)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	home, err := os.UserHomeDir()
	if err == nil {
		if got, want := Path(), filepath.Join(home, ".config", "go-vte", "config.toml"); got != want {
			t.Errorf("Path() without XDG = %q, want %q", got, want)
		}
	}
}

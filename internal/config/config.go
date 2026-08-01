// Package config loads user settings for go-vte from a flat key = value file
// (a TOML subset — our keys are all top-level). A missing file yields the
// built-in defaults; a malformed file yields the defaults for the bad keys plus
// an error the caller can log. It has no GPU or render dependency (colors are
// plain image/color.RGBA), so it is fully unit-testable.
package config

import (
	"errors"
	"fmt"
	"image/color"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Debug enables verbose config-loading logs (path, missing file, effective
// values). Read/parse errors are always logged regardless. Defaults from the
// GO_VTE_DEBUG environment variable.
var Debug = os.Getenv("GO_VTE_DEBUG") != ""

// debugf logs only when Debug is set.
func debugf(format string, args ...any) {
	if Debug {
		log.Printf("[config] "+format, args...)
	}
}

// Config holds all user-tunable settings. See CLAUDE.MD §6.
type Config struct {
	FontSize float64

	// FontSnap nudges the effective pixel size (FontSize × device scale) to the
	// nearest one whose character advance is a whole number of pixels, so every
	// column of the grid starts on a pixel boundary and text rasterizes uniformly.
	// See render.snapToWholeAdvance for why that matters. Off honours FontSize
	// exactly and accepts the softer, column-dependent rendering.
	FontSnap bool

	Foreground  color.RGBA
	Background  color.RGBA
	Cursor      color.RGBA
	SelectionFG color.RGBA
	SelectionBG color.RGBA
	Palette     [16]color.RGBA

	// Cursor presentation. CursorStyle is the shape used when the running app has
	// not asked for one via DECSCUSR; CursorOpacity is the alpha the cursor is
	// drawn with (0 = invisible, 1 = opaque, hiding the glyph beneath);
	// CursorBlink is the default blink, likewise overridable by DECSCUSR.
	CursorStyle     string // block | beam | underline
	CursorOpacity   float64
	CursorBlink     bool
	ScrollbackLines int
	IconFillRatio   float64

	// Key autorepeat. Holding a key re-sends it after KeyRepeatDelayMs, then
	// every KeyRepeatIntervalMs. A delay <= 0 disables autorepeat. Ebiten does
	// not surface OS autorepeat, so the terminal implements it itself.
	KeyRepeatDelayMs    int
	KeyRepeatIntervalMs int

	// Padding is the inset in logical pixels on each side of the terminal
	// content (scaled by the display device scale at render time).
	PaddingTop    int
	PaddingRight  int
	PaddingBottom int
	PaddingLeft   int

	// WindowDecorated controls the OS window border/title bar. false = borderless.
	WindowDecorated bool

	// Keys holds the configurable tab-management hotkeys.
	Keys Keybindings
}

// Binding is a parsed key chord: a normalized key token plus modifier flags.
type Binding struct {
	Key   string
	Ctrl  bool
	Shift bool
	Alt   bool
}

// Keybindings maps tab actions to their chords.
type Keybindings struct {
	NewTab   Binding
	CloseTab Binding
	NextTab  Binding
	PrevTab  Binding
}

// DefaultKeys returns the built-in hotkeys.
func DefaultKeys() Keybindings {
	return Keybindings{
		NewTab:   Binding{Key: "t", Ctrl: true, Shift: true},
		CloseTab: Binding{Key: "w", Ctrl: true, Shift: true},
		NextTab:  Binding{Key: "tab", Ctrl: true},
		PrevTab:  Binding{Key: "tab", Ctrl: true, Shift: true},
	}
}

// DefaultLight returns the built-in high-contrast light theme configuration.
// ANSI colors 0..15 are properly inverted and contrast-adjusted for light background (#f0eee6):
// color0 (Black) & color8 (Bright Black) are dark charcoal/slate to ensure comments and dark text
// have high contrast (>4.5:1 / >10:1 ratio).
func DefaultLight() Config {
	return Config{
		FontSize:    16,
		FontSnap:    true,
		Foreground:  rgb(0x1f, 0x23, 0x28),
		Background:  rgb(0xf0, 0xee, 0xe6),
		Cursor:      rgb(0x09, 0x69, 0xda),
		SelectionFG: rgb(0xf0, 0xee, 0xe6),
		SelectionBG: rgb(0x24, 0x29, 0x2e),
		Palette: [16]color.RGBA{
			rgb(0x1f, 0x23, 0x28), rgb(0xcf, 0x22, 0x2e), rgb(0x11, 0x63, 0x29), rgb(0x8d, 0x5b, 0x00),
			rgb(0x09, 0x69, 0xda), rgb(0x82, 0x50, 0xdf), rgb(0x1b, 0x7c, 0x83), rgb(0x6e, 0x77, 0x81),
			rgb(0x57, 0x60, 0x6a), rgb(0xa4, 0x0e, 0x26), rgb(0x1a, 0x7f, 0x37), rgb(0x63, 0x4c, 0x00),
			rgb(0x21, 0x8b, 0xff), rgb(0xa4, 0x75, 0xf9), rgb(0x31, 0x92, 0xaa), rgb(0x8c, 0x95, 0x9f),
		},
		CursorStyle:         "block",
		CursorOpacity:       0.6,
		CursorBlink:         false,
		ScrollbackLines:     10000,
		IconFillRatio:       0.85,
		KeyRepeatDelayMs:    500,
		KeyRepeatIntervalMs: 30,
		WindowDecorated:     true,
		Keys:                DefaultKeys(),
	}
}

// DefaultDark returns the built-in dark theme configuration.
func DefaultDark() Config {
	cfg := DefaultLight()
	cfg.Foreground = rgb(0xc9, 0xd1, 0xd9)
	cfg.Background = rgb(0x0d, 0x11, 0x17)
	cfg.Cursor = rgb(0x58, 0xa6, 0xff)
	cfg.SelectionFG = rgb(0x0d, 0x11, 0x17)
	cfg.SelectionBG = rgb(0x58, 0xa6, 0xff)
	cfg.Palette = [16]color.RGBA{
		rgb(0x48, 0x4f, 0x58), rgb(0xff, 0x7b, 0x72), rgb(0x3f, 0xb9, 0x50), rgb(0xd2, 0x99, 0x22),
		rgb(0x58, 0xa6, 0xff), rgb(0xbc, 0x8c, 0xff), rgb(0x39, 0xc5, 0xcf), rgb(0xb1, 0xba, 0xc4),
		rgb(0x6e, 0x76, 0x81), rgb(0xff, 0xa1, 0x98), rgb(0x56, 0xd3, 0x64), rgb(0xe3, 0xb3, 0x41),
		rgb(0x79, 0xc0, 0xff), rgb(0xd2, 0xa8, 0xff), rgb(0x56, 0xd4, 0xdd), rgb(0xf0, 0xf6, 0xfc),
	}
	return cfg
}

// Default returns the built-in configuration (the default high-contrast light theme).
func Default() Config {
	return DefaultLight()
}

// Path returns the config file location: $XDG_CONFIG_HOME/go-vte/config.toml,
// falling back to ~/.config/go-vte/config.toml.
func Path() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "go-vte", "config.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "go-vte", "config.toml")
	}
	return filepath.Join(home, ".config", "go-vte", "config.toml")
}

// Load reads the config at path. A missing file returns the defaults and no
// error; a malformed file returns a config with the defaults for the bad keys
// plus a descriptive error.
func Load(path string) (Config, error) {
	cfg := Default()
	debugf("reading %s", path)

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			debugf("no config file at %s; using defaults", path)
			return cfg, nil
		}
		log.Printf("[config] cannot read %s: %v; using defaults", path, err)
		return cfg, err
	}

	cfg, perr := parse(cfg, string(data))
	if perr != nil {
		log.Printf("[config] %s: %v", path, perr)
	} else {
		debugf("loaded %s (%d bytes)", path, len(data))
	}
	debugf("effective: font_size=%.0f snap=%t scrollback=%d icon_fill=%.2f cursor=%s/%.2f/blink=%t padding L%d/R%d/T%d/B%d",
		cfg.FontSize, cfg.FontSnap, cfg.ScrollbackLines, cfg.IconFillRatio, cfg.CursorStyle,
		cfg.CursorOpacity, cfg.CursorBlink,
		cfg.PaddingLeft, cfg.PaddingRight, cfg.PaddingTop, cfg.PaddingBottom)
	return cfg, perr
}

// parse applies key = value lines to cfg, accumulating per-line errors.
func parse(cfg Config, content string) (Config, error) {
	var errs []string
	for i, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(stripComment(raw))
		if line == "" {
			continue
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			errs = append(errs, fmt.Sprintf("line %d: missing '='", i+1))
			continue
		}
		if err := cfg.set(strings.TrimSpace(key), unquote(strings.TrimSpace(val))); err != nil {
			errs = append(errs, fmt.Sprintf("line %d: %v", i+1, err))
		}
	}
	if len(errs) > 0 {
		return cfg, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return cfg, nil
}

// set assigns a single key. On a bad value it leaves the field untouched (so the
// default survives) and returns an error.
func (c *Config) set(key, val string) error {
	switch {
	case key == "theme":
		switch strings.ToLower(val) {
		case "light":
			*c = DefaultLight()
			return nil
		case "dark":
			*c = DefaultDark()
			return nil
		default:
			return fmt.Errorf("invalid theme %q (expected \"light\" or \"dark\")", val)
		}
	case key == "font_size":
		return setFloat(&c.FontSize, val)
	case key == "font_snap":
		return setBool(&c.FontSnap, val)
	case key == "icon_fill_ratio":
		return setFloat(&c.IconFillRatio, val)
	case key == "scrollback_lines":
		return setInt(&c.ScrollbackLines, val)
	case key == "key_repeat_delay_ms":
		return setInt(&c.KeyRepeatDelayMs, val)
	case key == "key_repeat_interval_ms":
		return setInt(&c.KeyRepeatIntervalMs, val)
	case key == "padding_top":
		return setInt(&c.PaddingTop, val)
	case key == "padding_right":
		return setInt(&c.PaddingRight, val)
	case key == "padding_bottom":
		return setInt(&c.PaddingBottom, val)
	case key == "padding_left":
		return setInt(&c.PaddingLeft, val)
	case key == "cursor_style":
		return setCursorStyle(&c.CursorStyle, val)
	case key == "cursor_opacity":
		return setOpacity(&c.CursorOpacity, val)
	case key == "cursor_blink":
		return setBool(&c.CursorBlink, val)
	case key == "window_decorated":
		return setBool(&c.WindowDecorated, val)
	case key == "key_new_tab":
		return setBinding(&c.Keys.NewTab, val)
	case key == "key_close_tab":
		return setBinding(&c.Keys.CloseTab, val)
	case key == "key_next_tab":
		return setBinding(&c.Keys.NextTab, val)
	case key == "key_prev_tab":
		return setBinding(&c.Keys.PrevTab, val)
	case key == "foreground":
		return setColor(&c.Foreground, val)
	case key == "background":
		return setColor(&c.Background, val)
	case key == "cursor":
		return setColor(&c.Cursor, val)
	case key == "selection_foreground":
		return setColor(&c.SelectionFG, val)
	case key == "selection_background":
		return setColor(&c.SelectionBG, val)
	case strings.HasPrefix(key, "color"):
		idx, err := strconv.Atoi(strings.TrimPrefix(key, "color"))
		if err != nil || idx < 0 || idx > 15 {
			return fmt.Errorf("unknown color key %q", key)
		}
		return setColor(&c.Palette[idx], val)
	default:
		return fmt.Errorf("unknown key %q", key)
	}
}

func setFloat(dst *float64, val string) error {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fmt.Errorf("invalid number %q", val)
	}
	*dst = f
	return nil
}

func setInt(dst *int, val string) error {
	n, err := strconv.Atoi(val)
	if err != nil {
		return fmt.Errorf("invalid integer %q", val)
	}
	*dst = n
	return nil
}

func setBool(dst *bool, val string) error {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return fmt.Errorf("invalid boolean %q", val)
	}
	*dst = b
	return nil
}

func setColor(dst *color.RGBA, val string) error {
	c, err := parseHexColor(val)
	if err != nil {
		return err
	}
	*dst = c
	return nil
}

func setBinding(dst *Binding, val string) error {
	b, err := parseBinding(val)
	if err != nil {
		return err
	}
	*dst = b
	return nil
}

func parseBinding(s string) (Binding, error) {
	var b Binding
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), "+")
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if i < len(parts)-1 {
			switch p {
			case "ctrl", "control":
				b.Ctrl = true
			case "shift":
				b.Shift = true
			case "alt", "option":
				b.Alt = true
			default:
				return Binding{}, fmt.Errorf("unknown modifier %q in keybinding %q", p, s)
			}
			continue
		}
		if p != "tab" && len(p) != 1 {
			return Binding{}, fmt.Errorf("unknown key %q in keybinding %q", p, s)
		}
		b.Key = p
	}
	if b.Key == "" {
		return Binding{}, fmt.Errorf("empty keybinding %q", s)
	}
	return b, nil
}

func setOpacity(dst *float64, val string) error {
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return fmt.Errorf("invalid number %q", val)
	}
	if f < 0 || f > 1 {
		return fmt.Errorf("opacity %v out of range [0,1]", f)
	}
	*dst = f
	return nil
}

func setCursorStyle(dst *string, val string) error {
	switch val {
	case "block", "beam", "underline":
		*dst = val
		return nil
	default:
		return fmt.Errorf("invalid cursor_style %q", val)
	}
}

func parseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("invalid hex color %q", s)
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}, nil
}

func stripComment(line string) string {
	inQuote := false
	for i, r := range line {
		switch r {
		case '"':
			inQuote = !inQuote
		case '#':
			if !inQuote {
				return line[:i]
			}
		}
	}
	return line
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 0xff} }

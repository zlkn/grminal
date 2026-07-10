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

	Foreground  color.RGBA
	Background  color.RGBA
	Cursor      color.RGBA
	SelectionFG color.RGBA
	SelectionBG color.RGBA
	Palette     [16]color.RGBA

	CursorStyle     string // block | beam | underline
	ScrollbackLines int
	IconFillRatio   float64

	// Padding is the inset in logical pixels on each side of the terminal
	// content (scaled by the display device scale at render time).
	PaddingTop    int
	PaddingRight  int
	PaddingBottom int
	PaddingLeft   int
}

// Default returns the built-in configuration (the current light theme).
func Default() Config {
	return Config{
		FontSize:    16,
		Foreground:  rgb(0x42, 0x42, 0x42),
		Background:  rgb(0xf0, 0xee, 0xe6),
		Cursor:      rgb(0x20, 0xbb, 0xfc),
		SelectionFG: rgb(0xf0, 0xee, 0xe6),
		SelectionBG: rgb(0x42, 0x42, 0x42),
		Palette: [16]color.RGBA{
			rgb(0xd1, 0xd1, 0xd1), rgb(0xb8, 0x1a, 0x6b), rgb(0x1e, 0x76, 0x3c), rgb(0x8d, 0x5b, 0x00),
			rgb(0x01, 0x54, 0x93), rgb(0x75, 0x22, 0x8e), rgb(0x00, 0x74, 0x74), rgb(0x42, 0x42, 0x42),
			rgb(0x57, 0x60, 0x6a), rgb(0xb8, 0x1a, 0x6b), rgb(0x1e, 0x76, 0x3c), rgb(0x8d, 0x5b, 0x00),
			rgb(0x01, 0x54, 0x93), rgb(0x75, 0x22, 0x8e), rgb(0x00, 0x74, 0x74), rgb(0x08, 0x51, 0x57),
		},
		CursorStyle:     "block",
		ScrollbackLines: 10000,
		IconFillRatio:   0.85,
	}
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
	debugf("effective: font_size=%.0f scrollback=%d icon_fill=%.2f cursor=%s padding L%d/R%d/T%d/B%d",
		cfg.FontSize, cfg.ScrollbackLines, cfg.IconFillRatio, cfg.CursorStyle,
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
	case key == "font_size":
		return setFloat(&c.FontSize, val)
	case key == "icon_fill_ratio":
		return setFloat(&c.IconFillRatio, val)
	case key == "scrollback_lines":
		return setInt(&c.ScrollbackLines, val)
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

func setColor(dst *color.RGBA, val string) error {
	c, err := parseHexColor(val)
	if err != nil {
		return err
	}
	*dst = c
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

// parseHexColor parses "#rrggbb" (or "rrggbb") into an opaque RGBA.
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

// stripComment removes a trailing '#' comment that is not inside a quoted
// string, so quoted hex colors ("#rrggbb") survive.
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

// unquote strips a single pair of surrounding double quotes.
func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{R: r, G: g, B: b, A: 0xff} }

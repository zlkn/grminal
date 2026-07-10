// Package fonts embeds the bundled terminal fonts so the binary is
// self-contained (no runtime font-file dependency).
package fonts

import _ "embed"

// JetBrainsMono is JetBrains Mono Nerd Font (Mono variant) Regular: a monospace
// font with programming ligatures (calt) plus the Nerd Font icon set, with every
// glyph — icons included — constrained to a single cell so the terminal grid
// stays uniform. (The non-Mono variant has byte-identical ligature glyphs but
// double-width icons, which would break the single-cell grid, so Mono is used.)
//
//go:embed JetBrainsMonoNerdFontMono-Regular.ttf
var JetBrainsMono []byte

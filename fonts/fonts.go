// Package fonts embeds the bundled terminal fonts so the binary is
// self-contained (no runtime font-file dependency).
package fonts

import _ "embed"

// JetBrainsMono is JetBrains Mono Regular, a monospace font with programming
// ligatures (calt) — used as the terminal's default face.
//
//go:embed JetBrainsMono-Regular.ttf
var JetBrainsMono []byte

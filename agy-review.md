# Comprehensive Code Review Report: `go-vte`

## 1. Executive Summary & System Architecture Assessment

`go-vte` is a GPU-accelerated terminal emulator written in pure Go (`CGO_ENABLED=0`) targeting Linux/macOS. It uses `ebiten/v2` for GPU rendering, `text/v2` with `sfnt` for OpenType text shaping and ligature rendering, and `creack/pty` for OS pseudo-terminal integration.

### Architectural Alignment against `CLAUDE.MD`
- **Layered Architecture**: **Excellent**. The codebase strictly enforces layer separation. The VTE parser (`vte`) has zero dependencies on graphics libraries or PTY implementations; `layout` is pure geometric arithmetic; `pane` handles PTY I/O and grid synchronization; `render` handles GPU text drawing; and `app` coordinates window events and tabs.
- **Pure Go & Zero-Cgo Guarantee**: **Pass**. Verified no Cgo calls or native library bindings exist.
- **Demand-Driven Rendering**: **Pass**. Render updates are gated by `dirty atomic.Bool` in [run.go](file:///home/yzolkin/Personal/go-vte/internal/app/run.go#L70), preventing GPU work when idle while blitting double-buffered offscreen frames to avoid display ghosting under vsync.
- **Font Snapping & Ligature Preservation**: **Pass**. Character advances are snapped to integer pixels via `snapToWholeAdvance` in [draw.go](file:///home/yzolkin/Personal/go-vte/internal/render/draw.go#L141), preserving crisp rasterization and preventing drift across cell boundaries.

---

## 2. Critical & High Priority Issues (Correctness & Edge Cases)

### 🔴 Issue 1: UTF-8 Stream Fragmentation in `vte.Parser`
- **Location**: [internal/vte/parser.go:L79-L86](file:///home/yzolkin/Personal/go-vte/internal/vte/parser.go#L79-L86)
- **Impact**: **High (Data Corruption)**.
- **Description**: In `Parser.Write(b []byte)`, printable runes are decoded using `utf8.DecodeRune(b[i:])`. When a multi-byte UTF-8 rune (e.g. `'€'` = `3 bytes`) is split across chunk boundaries of PTY reads, `b[i:]` contains an incomplete UTF-8 sequence. `utf8.DecodeRune` returns `(utf8.RuneError, 1)`. The parser emits `\uFFFD` into the terminal grid, fails to buffer the incomplete bytes, and then emits a second `\uFFFD` when the next chunk arrives.
- **Remediation**:
  Buffer incomplete trailing UTF-8 bytes in `Parser` across `Write()` invocations:
  ```go
  // In Parser struct (parser.go):
  utfBuf [4]byte
  utfLen int

  // In Parser.Write:
  if p.utfLen > 0 {
      // Prepend buffered bytes before decoding
  }
  ```

---

### 🔴 Issue 2: Initial PTY Startup Size Race (0×0 Grid)
- **Location**: [internal/app/run.go:L84-L100](file:///home/yzolkin/Personal/go-vte/internal/app/run.go#L84-L100), [internal/pane/ospty.go:L31](file:///home/yzolkin/Personal/go-vte/internal/pane/ospty.go#L31)
- **Impact**: **Medium (CLI Application Glitches)**.
- **Description**: `game` initializes with `cols = 0` and `rows = 0`. During `Run()`, Ebiten executes `g.Update()` *before* `g.LayoutF()`. On the very first frame, `reconcilePanes()` calls `pane.StartShell(0, 0)`. The PTY is spawned with `0×0` window dimensions. Immediately after, `LayoutF()` measures the window and issues a `p.Resize(cols, rows)`, firing a `SIGWINCH` signal to the child shell right after boot.
- **Remediation**: Pre-calculate initial columns and rows from `initialWidth` (900px) and `initialHeight` (600px) when creating `Renderer` inside `Run()`.

---

### 🟠 Issue 3: SGR & CSI Colon Parameter Bailing (`:`)
- **Location**: [internal/vte/parser.go:L172-L207](file:///home/yzolkin/Personal/go-vte/internal/vte/parser.go#L172-L207)
- **Impact**: **Medium (ANSI Parsing Compatibility)**.
- **Description**: Modern CLI applications (`bat`, `git`, `neovim`, `eza`) format truecolor and extended SGR parameters using colon delimiters (e.g. `\x1b[38:2::R:G:Bm` or `\x1b[4:3m`). In `Parser.csi(c)`, encountering a colon (`c == ':'`, 0x3A) matches `default: p.state = stateGround`, which immediately aborts sequence parsing and prints the rest of the ANSI escape as garbage.
- **Remediation**:
  Accept `:` as a subparameter delimiter in `csi()` alongside `;`, or handle ISO/IEC 8613-6 subparameter parsing in `applySGR()`.

---

### 🟠 Issue 4: Incomplete OSC Escape Abort (`stateOSCEsc`)
- **Location**: [internal/vte/parser.go:L94-L98](file:///home/yzolkin/Personal/go-vte/internal/vte/parser.go#L94-L98)
- **Impact**: **Low-Medium (Parsing Edge Case)**.
- **Description**: In `Parser.Write()`, if `stateOSCEsc` is active (saw `ESC` inside an OSC sequence) and the next byte is NOT `\`, `finishOSC()` resets `p.state = stateGround`. However, the current byte `c` that broke out of `stateOSCEsc` is skipped without being processed in `stateGround` or `stateEscape`.
- **Remediation**: Re-evaluate byte `c` after exiting `stateOSCEsc` if it is not `\`.

---

## 3. Concurrency & Thread Safety Analysis

### 🟡 Issue 5: Unsynchronized `onChange` Callback Access
- **Location**: [internal/pane/pane.go:L59](file:///home/yzolkin/Personal/go-vte/internal/pane/pane.go#L59), [L183-L185](file:///home/yzolkin/Personal/go-vte/internal/pane/pane.go#L183-L185)
- **Impact**: **Low (Potential Race Condition)**.
- **Description**: `SetChangeHook(fn)` writes `p.onChange` without acquiring `p.mu`. Meanwhile, `Pane.Run()` reads and executes `p.onChange()` outside `p.mu` after processing PTY chunks. Calling `SetChangeHook` while `Run()` is active causes a data race on `p.onChange`.
- **Remediation**:
  Acquire `p.mu` inside `SetChangeHook` and read `p.onChange` under `p.mu` (or store as `atomic.Value`).

### ✅ Thread Safety Strengths
- **Grid Snapshotting**: The PTY drain goroutine mutates `vte.Grid` under `Pane.mu`. The Ebiten rendering loop reads immutable `vte.Snapshot` structs created under `Pane.mu.Lock()`. This cleanly eliminates data races between PTY read loops and GPU rendering.

---

## 4. Input Layer & Terminal Protocol Audit

### 🟠 Issue 6: Missing Non-Letter Control Keys (`Ctrl+[`, `Ctrl+]`, `Ctrl+\`)
- **Location**: [internal/app/input.go:L318-L325](file:///home/yzolkin/Personal/go-vte/internal/app/input.go#L318-L325)
- **Impact**: **Medium (Terminal Utility Usability)**.
- **Description**: `appendSpecialKeys` only iterates over `ebiten.KeyA` through `ebiten.KeyZ` when `ctrl` is true. Essential terminal control shortcuts such as `Ctrl+[` (`ESC` = `0x1B`), `Ctrl+]` (`0x1D`), `Ctrl+\` (`SIGQUIT` = `0x1C`), `Ctrl+^` (`0x1E`), and `Ctrl+_` (`0x1F`) are not encoded.
- **Remediation**: Add explicit mappings for bracket/punctuation control chords:
  ```go
  if ctrl {
      switch {
      case inpututil.IsKeyJustPressed(ebiten.KeyBracketLeft):  b = append(b, 0x1b)
      case inpututil.IsKeyJustPressed(ebiten.KeyBackslash):    b = append(b, 0x1c)
      case inpututil.IsKeyJustPressed(ebiten.KeyBracketRight): b = append(b, 0x1d)
      }
  }
  ```

---

### 🟡 Issue 7: Key Autorepeat Payload Clumping
- **Location**: [internal/app/input.go:L123-L132](file:///home/yzolkin/Personal/go-vte/internal/app/input.go#L123-L132)
- **Impact**: **Low (Input Glitch under High Frame Time)**.
- **Description**: `primaryPressedKey()` returns the last non-modifier key pressed in a tick, but `g.repeat.arm(key, buf)` captures the entire `buf` slice (which may contain multiple input characters if typed quickly within 1 tick). If held, `repeatBytes()` re-emits all characters in `buf` repeatedly.

---

## 5. Performance & Allocation Hotspots

1. **Text Glyph Allocation in `drawRunText`**:
   - [internal/render/draw.go:L262](file:///home/yzolkin/Personal/go-vte/internal/render/draw.go#L262): `text.AppendGlyphs(nil, run.Text, r.face, nil)` allocates a new slice of `text.Glyph` on every drawn text run.
   - **Optimization**: Maintain a pre-allocated `glyphBuf []text.Glyph` field on `Renderer` to reuse slice capacity across runs.
2. **Scrollback History View Snapshotting**:
   - [internal/pane/pane.go:L156-L163](file:///home/yzolkin/Personal/go-vte/internal/pane/pane.go#L156-L163): `SnapshotScrolled()` allocates 2D cell arrays (`make([][]vte.Cell, offset)`) every frame when viewing history.

---

## 6. Summary of Test Coverage & Code Quality

| Component | Status | Test Quality & Coverage |
| :--- | :--- | :--- |
| `fonts` | ✅ Pass | Monospace & icon coverage verified via unit tests |
| `internal/config` | ✅ Pass | Table-driven TOML parser tests (hex, bounds, bindings) |
| `internal/vte` | ✅ Pass | ANSI state machine, autowrap, BCE, reflow, mouse reporting |
| `internal/scrollback` | ✅ Pass | Ring buffer eviction & indexing tests |
| `internal/pane` | ✅ Pass | Concurrency tests with fake PTY |
| `internal/layout` | ✅ Pass | Flexbox grid calculation & tiling tests |
| `internal/render` | ✅ Pass | Headless unit tests for run splitting & snapping + GPU tests |
| `internal/app` | ✅ Pass | Tab management & input state tests |

---

## 7. Actionable Priority Roadmap

1. **Phase 1 (Bug Fixes)**:
   - Implement partial UTF-8 byte buffering in `vte.Parser` ([parser.go](file:///home/yzolkin/Personal/go-vte/internal/vte/parser.go#L79)).
   - Fix initial window scale/grid sizing in `Run()` before spawning shell PTY ([run.go](file:///home/yzolkin/Personal/go-vte/internal/app/run.go#L97)).
   - Support `Ctrl+[` and non-alphabetic control combinations in `appendSpecialKeys` ([input.go](file:///home/yzolkin/Personal/go-vte/internal/app/input.go#L318)).
   - Add SGR colon subparameter support (`:`) in `csi()` ([parser.go](file:///home/yzolkin/Personal/go-vte/internal/vte/parser.go#L203)).
2. **Phase 2 (Performance & Ergonomics)**:
   - Reuse `text.Glyph` slice buffers in `Renderer.eachGlyph` ([draw.go](file:///home/yzolkin/Personal/go-vte/internal/render/draw.go#L262)).
   - Lock `p.onChange` in `SetChangeHook` ([pane.go](file:///home/yzolkin/Personal/go-vte/internal/pane/pane.go#L59)).

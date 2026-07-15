# First Build: Wiring the Tested Core into a Runnable Terminal

The entire **model** is already TDD-covered and green under `go test -race`:
parser (CSI/SGR/OSC), grid, scrollback ring, layout, tabs/AppState, and the
thread-safe `vte.Snapshot`. What remains is the **on-display integration glue** —
GPU drawing, the real PTY, and the Ebitengine main loop. This glue is verified by
*running the app*, not by unit tests (the Stage-0 spike showed `ebiten` pixel
readback panics headless), so build and test it on a machine with a display.

Do this work on a branch.

---

## 1. Add the runtime dependencies

```fish
go get github.com/hajimehoshi/ebiten/v2@latest
go get github.com/creack/pty@latest
# plus a monospace/ligature .ttf embedded under internal/render/assets/
```

---

## 2. Fill the four glue seams (in dependency order)

### a. `render.Draw` — snapshot → pixels
The only substantial new piece. Turns a `vte.Snapshot` into a drawn frame:

- Load a `text/v2.GoTextFace` once; measure cell width/height from the font.
- For each row: `SplitRuns(snap.Row(y))` →
  - draw each run's **background** rect (`vector.DrawFilledRect`),
  - draw the run's **text** (`text.Draw`) at `x = run.Col * cellW`.
- Draw the cursor block at `(snap.CurX, snap.CurY)`.
- Clip each pane with `screen.SubImage(rect)` — hardware clipping per `CLAUDE.MD`.

Reuses the already-tested `render.SplitRuns` and `vte.Snapshot`; no model changes.

### b. Real PTY adapter — `internal/pane/pty_creack.go` (build-tagged)
Implements the **existing** `PTY` seam, so `NewPane` is unchanged:

```go
ptmx, _ := pty.Start(exec.Command(os.Getenv("SHELL")))
// Read/Write pass through ptmx; Resize -> pty.Setsize(ptmx, &pty.Winsize{Rows, Cols})
```

Exercise it only with a build-tagged integration test — never in the unit suite.

### c. `app.Run` — the Ebitengine `Game`
Wire tabs + layout + panes into the loop:

- `Update()`:
  - `inpututil` keys → `App.HandleKey(...)` for tab hotkeys;
  - otherwise encode the key → `activePane.Write(...)`;
  - mouse click → `layout.PaneAt(x, y)` to move input focus.
- `Draw(screen)`: for each `id, rect := range layout.Rects()`,
  `render.Draw(screen.SubImage(rect), pane[id].Snapshot())`.
- `Layout(w, h)`: return the window size; on change call `pane.Resize(cols, rows)`.
- Spawn each pane's `Run()` in a **goroutine** — the mutex from step 10 makes the
  concurrent grid access safe.

### d. Wire scrollback into the grid
In `grid.scrollUp` (TODO already present), push the evicted top row into the
`scrollback.Ring` instead of dropping it.

---

## 3. Build & run

> **CGO note (correction to CLAUDE.MD):** Ebitengine's desktop-Linux backend
> uses glfw/X11/OpenGL via **cgo**, so `CGO_ENABLED=0` does *not* build the GUI on
> Linux (glfw symbols end up undefined). Build with cgo (the default). You need a
> C compiler and the X11/GL dev headers:
>
> ```fish
> # Debian/Ubuntu build deps
> sudo apt install gcc libgl1-mesa-dev xorg-dev
> ```

```fish
go build -o bin/go-vte ./cmd/go-vte
./bin/go-vte
```

---

## What stays tested vs. verified-by-running

| Layer | How it's checked |
|-------|------------------|
| parser, grid, scrollback, layout, tabs, snapshot | unit + golden + `-race` (done) |
| `render.SplitRuns` tokenization | unit (done) |
| `render.Draw` GPU calls | run the app (visual) |
| PTY adapter | build-tagged integration test + running the app |
| `app.Run` main loop / input | run the app (interactive) |

---

## Suggested verification once running

1. Shell prompt renders; typing echoes; `ls`, `vim`, `htop` display correctly
   (colors, cursor movement, clears).
2. Resize the window → grid reflows, PTY `SIGWINCH` reaches the shell.
3. Tab hotkeys (`Ctrl+Shift+T`/`W` create/close, `Ctrl+Tab`/`Ctrl+Shift+Tab` cycle) switch/create/close tabs.
4. Splits tile without gaps; click moves focus to the clicked pane.
5. Scrollback retains lines that scroll off the top.
6. Sanity: `go vet ./...` and `go test -race ./...` still green.

# Why ligatures look softer in go-vte than in darktile

Investigation notes, comparing `go-vte` against `liamg/darktile` on the same
machine and the same display. Both are Ebitengine terminals, so "darktile looks
crisper" reads like a go-vte defect. It mostly isn't: the two draw different
things, at different resolutions, and only one of them draws a real ligature.

Measurement environment: eDP-1 3072x1728 on a 310x170mm panel, GNOME/Wayland,
`Xft.dpi=192`, so `ebiten.Monitor().DeviceScaleFactor()` returns **2** (measured,
not assumed). go-vte config `font_size = 16`, `font_snap` defaulted on.

---

## 1. darktile has no OpenType ligatures at all

`darktile/internal/app/darktile/gui/render/ligatures.go:10` is a hardcoded map of
ten sequences onto single precomposed Unicode characters:

```go
var ligatures = map[string]rune{
    ":=": '≔', "===": '≡', "!=": '≠', "!==": '≢', "<=": '≤',
    ">=": '≥', "=>": '⇒', "->": '→', "<-": '←', "<>": '≷',
}
```

`handleLigatures` (`ligatures.go:38`) draws that one glyph from MesloLGS NF,
centred across the cell pair, at an integer pixel position. Meslo has no `liga`
table and darktile runs no shaper — ebiten's old `text` package caches one bitmap
per rune and blits it at integer coordinates forever.

go-vte does the real thing: HarfBuzz shaping via `text/v2`, so `!=` becomes one
**wide two-cell JetBrains Mono glyph** built from long thin diagonals and
horizontal bars. That is the hardest shape there is to rasterize cleanly, and
it is a different rendering problem from "draw ≠ in one cell".

Consequence: any comparison of "their ligatures" against "our ligatures" is
comparing a 11x11 math symbol against a 33x22 multi-cell glyph.

---

## 2. darktile renders at half resolution and is pixel-doubled

This is the dominant visual difference, and it applies to *all* text, not just
ligatures.

| | go-vte | darktile |
|---|---|---|
| Layout hook | `LayoutF` returns `outside x scale` (`internal/app/run.go:310`) | `Layout` returns the device-independent size (`gui/resize.go:8`) |
| Offscreen size | == framebuffer (3072x1728) | half of it |
| Ebiten screen scale | 1.0 | 2.0 (`vendor/.../uicontext.go:127`) |
| Final blit | none | `filterScreen` sharp-bilinear (`uicontext.go:233`) |
| Glyph em size | 30 physical px | 18 logical px = **36 physical px** |

`filterScreen` clamps the interpolation rate so that at an integer scale each
source pixel becomes a hard-edged NxN block. darktile's text is therefore a
chunky *bitmap*: every edge is a hard step, no partial-coverage gradients
anywhere. That reads as "crisp". It is also 20% larger than ours.

go-vte renders natively at 252 dpi with true antialiasing — physically the more
accurate result, and the one that will still look right on a 1x display, but it
has real grey edge pixels, and on a light theme those read as "soft".

**This is not a bug to copy.** Matching darktile would mean throwing away half
the resolution on a HiDPI screen.

---

## 3. Neither library actually hints

darktile calls `opentype.NewFace(..., Hinting: font.HintingFull)`
(`font/manager.go:108`), which looks like grid-fitting. It is not:
`golang.org/x/image/font/sfnt` states outright *"This implementation does not
support hinting"* (`sfnt.go:611`, and see the TODOs at `sfnt.go:1400`, `:1438`).
`HintingFull` there only rounds **advance and metrics** to whole pixels — which
is why darktile's advance comes out at exactly `11.000px`.

go-vte reaches the same end by a different route: `snapToWholeAdvance`
(`internal/render/draw.go:141`) plus `math.Ceil` on the cell.

So neither side grid-fits *stems*. Nothing in the Go font stack executes
TrueType hinting bytecode, JetBrains Mono's own hinting instructions included.
go-vte just suffers more from it, for the reason in §4.

---

## 4. What the pixels actually do

Rasterized through each project's own pipeline (ASCII coverage, `%` ~= 0.9
coverage, `+` ~= 0.6, `.` ~= 0.1):

```
go-vte  != @ 30px em            darktile ≠ @ 18px em     ... after the 2x upscale
|:+++++++++++++%#+++++++++:|    | %%%%%%%%% |            |  %%%%%%%%%%%%%%%%%%  |
|=%%%%%%%%%%%%%%%%%%%%%%%%=|    | ...:##... |            |  %%%%%%%%%%%%%%%%%%  |
|:+++++++++++++%#+++++++++:|                             |  ......::####......  |
     0.6 / 0.9 / 0.6                0.9 / 0.15               two hard 0.9 rows
```

go-vte's crossbar is ~2.1px thick and straddles the pixel grid, so its ink
spreads across **three rows with no fully covered row**. Same total ink,
smeared over 50% more height, with both edges at partial coverage. That is the
blur — it is vertical, and `font_snap` does nothing about it.

darktile's bar is one ~0.9 row plus a faint neighbour, then doubled into two
hard 0.9 rows.

Contrast makes it worse for us: our default theme is `#424242` on `#f0eee6`.
Partial-coverage pixels blended in non-linear sRGB look washed out on a light
background, in a way darktile's `#c5c8c6` on `#000000` never does.

---

## 5. `font_snap` optimizes the wrong axis for this symptom

`snapToWholeAdvance` nudges the effective size to the nearest one whose
*horizontal advance* is a whole number of pixels. It fixes per-column raster
drift (real, measured, documented in `CLAUDE.MD` §3). It says nothing about
where horizontal bars land vertically — and at the current settings it picks the
worse of the two candidates.

Measured, `!=` at device scale 2:

| physical px | source | `!=` crossbars |
|---|---|---|
| 30 | `font_size 16`, snap **on** (current) | 0.6 / 0.9 / 0.6 — three soft rows |
| 32 | `font_size 16`, snap off | one bar soft, one crisp |
| 35 | `font_size 18`, snap on | two solid rows |
| 36 | `font_size 18`, snap off | two solid 0.9 rows — clean |

So `font_snap` trades vertical stem alignment for horizontal advance alignment,
and at `font_size 16` on a 2x display that trade currently loses.

---

## 6. Options, cheapest first

1. **`font_size = 18`.** Puts both crossbars on the grid at scale 2. One config
   line, no code. Verified visually and in the raster dump.
2. **Gamma-corrected text blending.** Composite glyph coverage with a gamma
   ramp instead of straight sRGB alpha. This is what FreeType/CoreText do and it
   is the standard fix for washed-out dark-on-light antialiasing. Helps all
   text, not only ligatures.
3. **Teach `font_snap` about the vertical axis.** Score candidate sizes by stem
   alignment (fraction of ink pixels above ~0.9 coverage) as well as by advance
   integrality, and pick the best combined score. More faithful to the intent of
   the knob than the current advance-only rule.
4. **Real grid-fitting.** Not available: no Go rasterizer executes TT hinting.
   Would require an autohinter or Cgo/FreeType, both against the project's
   Pure-Go constraint.

Do **not** "fix" this by rendering at logical resolution the way darktile does.
It only looks better because it is a 2x-magnified bitmap.

---

## 7. Reproducing

`DeviceScaleFactor`, cell metrics and glyph coverage all need a live Ebiten
loop (pixel readback panics headless — see `CLAUDE.MD` §4). The pattern is the
one already used by `internal/render/ligature_gpu_test.go`: run
`ebiten.RunGame`, do the work inside `Draw`, return `ebiten.Termination`.

Existing coverage of the neighbouring invariant:

```fish
GPUTEST=1 go test ./internal/render -run LigatureRaster -v   # font_snap uniformity
```

To re-measure the vertical story, dump `g.Image` alpha from
`text.AppendGlyphs(nil, "!=", face, nil)` across sizes 24..40 and count pixels
above 0.9 coverage. There is currently **no test guarding vertical stem
alignment** — §6.3 would be the place to add one.

---

## Summary

- darktile's "ligatures" are substituted Unicode symbols, not shaped ligatures.
- darktile renders at half resolution and gets pixel-doubled, which fakes
  crispness; go-vte renders natively at 2x and shows honest antialiasing.
- Neither hints; go-vte's ligature crossbars land badly at 30px em specifically.
- `font_size = 18` is the cheap fix; gamma-correct blending is the real one.

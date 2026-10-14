# a-git explainer

An animated explainer of how a-git works, written in plain JavaScript on a
canvas. The same code drives the interactive player (`index.html`) and the
rendered video (`a-git-explainer.mp4`).

```sh
# Watch it: open docs/explainer/index.html in a browser.

# Render the MP4 (headless Chromium + ffmpeg):
node scripts/explainer/render.mjs --out docs/explainer/a-git-explainer.mp4

# Inspect frames while editing a scene (scene-local seconds):
node scripts/explainer/snap.mjs --scene changesets --times 3,9.5 --out /tmp/snaps
node scripts/explainer/snap.mjs --scene 04 --sheet 8 --debug --out /tmp/snaps
node scripts/explainer/snap.mjs --all --sheet 6 --out /tmp/snaps
```

`--debug` draws the safe area (magenta) and the scene-local time. `snap.mjs`
exits non-zero and prints `PAGE ERRORS` if a script throws.

## How it fits together

- `engine.js` defines `AGX`: the palette, fonts, a `Graphics` helper
  (`g.text`, `g.box`, `g.card`, `g.chip`, `g.node`, `g.arrow`, `g.packet`,
  `g.code`, `g.terminal`, `g.icon`, ...) and `renderAt(canvas, t)`.
- Each file in `scenes/` calls `AGX.scene({ id, order, title, duration,
  captions, draw(g, t, d) })`. Scenes play in `order`.
- `index.html` lists the scene scripts. The render and snapshot tools read
  that list, so it is the single source of truth for scene order.

## Rules for scenes

- **Deterministic.** `draw` must be a pure function of `t`. Never use
  `Math.random`, `Date` or `performance.now`. Use `g.hash(seed)` and
  `g.rand(seed)` instead.
- **Timing.** Animate with `g.at(start, dur, ease)` (0 to 1 progress),
  `g.vis(start, end)` (fade in, optional fade out) and `g.stagger(i, start,
  step, dur)`. The engine fades each scene in and out over 0.45 s at the
  boundaries.
- **Layout.** The canvas is 1920×1080. Keep content inside the safe area
  (x 110–1810, y 150–890). The chapter bar sits above it and captions sit
  below. Section headings use `g.heading(text)` at y 232.
- **Type.** Body text is at least 18 px and labels at least 17 px, because
  the video is often watched small. Use `display` for headings, `body` for
  prose, and `mono` for hashes, paths, commands and code.
- **Color meaning.** These are consistent across scenes:

  | Color | Meaning |
  |---|---|
  | teal | the server and refs |
  | amber | content-addressed objects and hashes |
  | blue | clients, agents, working copies |
  | coral | deletions, conflicts, failures, denials |
  | mint | additions, success, approvals, cache hits |
  | violet | derived data, reads |

- **Captions.** Each caption is on screen until the next one starts. Keep a
  caption to two lines (about 170 characters) and give the viewer about
  15 characters per second to read it. The visuals should show what the
  caption is talking about while it is on screen.
- **Accuracy.** Everything on screen must match the a-git implementation:
  command names and output formats, endpoint paths, header names, derived
  data kinds, file names in `.agit/`, and the REVIEWERS syntax.

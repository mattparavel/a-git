// Scene 5: derived data. Eager kinds (tree, graph) are computed when a
// changeset arrives; lazy kinds (history, renames, blame) on first request.
// Also shows the graph's skip list behind main~N.
AGX.scene({
  id: 'derived',
  order: 5,
  title: 'Derived data',
  duration: 19,
  captions: [
    { at: 0, text: 'Derived data is computed from a changeset plus what was already derived for its ancestors.' },
    { at: 6.0, text: 'Tree and graph are derived eagerly, on arrival. Skip pointers turn main~5 into two hops.' },
    { at: 12.9, text: 'History and blame are derived lazily, the first time someone asks, and then stored.' },
  ],
  draw(g, t) {
    const C = g.C;
    const V = C.violet;
    const C2 = 6.0, C3 = 12.9; // caption starts
    g.heading('Derived data, computed from ancestors', { alpha: g.vis(0) });

    const n = 8;
    const x0 = 230, dx = 205, y = 650;
    const X = (i) => x0 + i * dx;
    const arrive = g.at(C2 + 0.3, 1.0, 'out'); // the newest changeset slides in

    // A derived-data chip. p = 0: not derived yet (dashed outline);
    // p = 1: derived and stored (filled). s scales it around its centre.
    const kindChip = (label, x, cy, o) => {
      const size = o.size || 17;
      const p = o.p === undefined ? 1 : o.p;
      const padX = Math.round(size * 0.6), h = Math.round(size * 1.6);
      const w = g.measure(label, { size, font: 'mono', weight: 500 }) + padX * 2;
      const left = o.align === 'left' ? x : x - w / 2;
      const cx = left + w / 2;
      g.transform({ scale: o.scale || 1, cx, cy }, () => {
        if (p < 1) {
          g.box(left, cy - h / 2, w, h, { stroke: g.rgba(V, 0.55 * (1 - p)), dash: [4, 4], lineWidth: 1.5, radius: h / 2 });
        }
        if (p > 0) {
          g.box(left, cy - h / 2, w, h, { fill: g.rgba(V, 0.16 * p), stroke: g.rgba(V, 0.75 * p), lineWidth: 1.5, radius: h / 2 });
        }
        const ta = o.textAlpha === undefined ? 0.65 + 0.35 * p : o.textAlpha;
        g.text(label, left + padX, cy + size * 0.36, { size, font: 'mono', weight: 500, color: g.rgba(V, ta) });
      });
      return w;
    };

    // Legend: derived data is violet; eager is filled from the start, lazy is
    // an outline until someone asks for it.
    g.alpha(g.vis(0.4), () => {
      const ew = kindChip('eager', 110, 312, { size: 20, align: 'left' });
      const et = g.text('tree, graph: computed when a changeset is created', 110 + ew + 16, 319, { size: 21, color: C.muted });
      const lx = 110 + ew + 16 + et + 70;
      const lw = kindChip('lazy', lx, 312, { size: 20, align: 'left', p: 0, textAlpha: 1 });
      g.text('history, renames, blame: computed on first request, then stored', lx + lw + 16, 319, { size: 21, color: C.muted });
    });

    // Chain line.
    g.alpha(g.vis(0.3), () => {
      g.line(X(0), y, X(n - 2), y, { color: C.line, width: 4 });
      g.line(X(n - 2), y, g.lerp(X(n - 2), X(n - 1), arrive), y, { color: C.line, width: 4 });
    });

    // Lazy history fill order: walk back from c7 to the first derived
    // ancestor (c4), then derive c5, c6, c7 oldest first.
    const fill0 = C3 + 2.3;
    const histDerivedAt = (i) => (i <= 4 ? -1 : fill0 + (i - 5) * 0.55);

    for (let i = 0; i < n; i++) {
      const newest = i === n - 1;
      const xi = newest ? g.lerp(X(i) + 120, X(i), arrive) : X(i);
      const a = newest ? arrive : g.stagger(i, 0.3, 0.08, 0.4);
      g.alpha(a, () => {
        if (newest) g.circle(xi, y, 22, { fill: C.bg });
        g.circle(xi, y, 22, { fill: newest ? g.rgba(C.amber, 0.25) : C.bg, stroke: C.amber, lineWidth: 4 });
        g.text('c' + i, xi, y + 58, { size: 20, font: 'mono', color: C.muted, align: 'center' });

        // Eager kinds: tree and graph.
        const eagerA = newest ? g.at(C2 + 1.4, 0.4, 'out') : 1;
        g.alpha(eagerA, () => {
          kindChip('tree', xi, y - 70, {});
          kindChip('graph', xi, y - 112, {});
        });
        // Lazy kind: history, an outline until it is derived.
        const at = histDerivedAt(i);
        const p = at < 0 ? 1 : g.at(at, 0.35, 'out');
        const bump = at < 0 ? 0 : g.at(at, 0.45, 'pulse');
        kindChip('history', xi, y - 154, { p, scale: 1 + 0.12 * bump });
      });
    }

    // Caption 1: c6's values come from c6 itself plus what its ancestors
    // already have.
    g.alpha(g.vis(2.0, 5.6), () => {
      g.circle(X(6), y, 31, { stroke: g.rgba(V, 0.9), lineWidth: 3 });
      g.arrow(X(5) + 44, y - 112, X(6) - 44, y - 112, { color: V, width: 3, headSize: 11, progress: g.at(2.2, 0.6) });
      g.arrow(X(5) + 38, y - 70, X(6) - 38, y - 70, { color: V, width: 3, headSize: 11, progress: g.at(2.5, 0.6) });
      g.alpha(g.vis(2.8), () => {
        g.text("c6's tree and graph: from c6 + its ancestors' values", X(6), y + 108, { size: 20, color: V, align: 'center' });
      });
    });

    // Caption 2a: the new changeset gets its eager kinds on arrival.
    g.alpha(g.vis(C2 + 1.5, C2 + 3.6), () => {
      g.text('new changeset: tree and graph derived on arrival', X(n - 1), y + 108, { size: 20, color: V, align: 'right' });
    });

    // Caption 2b: main~5 via the graph's skip list. FirstParentAncestor takes
    // the low bit first: skip 1 from the tip (c7 -> c6), then c6's skip 4
    // (c6 -> c2).
    const skipEnd = C3;
    g.alpha(g.vis(C2 + 3.4, skipEnd), () => {
      g.chip('main', X(n - 1) + 36, y, { color: C.teal, size: 18 });
    });
    const hops = [
      { from: n - 1, to: n - 2, k: 1, bend: -60, start: C2 + 3.7, dur: 0.6, ly: y + 80, ex: 16, ey: 18 },
      // Ends just outside the main~5 halo drawn around c2.
      { from: n - 2, to: 2, k: 4, bend: -190, start: C2 + 4.4, dur: 0.9, ly: y + 144, ex: 26, ey: 24 },
    ];
    g.alpha(1 - g.at(skipEnd, 0.4, 'in'), () => {
      hops.forEach((h) => {
        const u = g.at(h.start, h.dur);
        if (u <= 0) return;
        const pts = g.arrow(X(h.from) - 16, y + 18, X(h.to) + h.ex, y + h.ey, { color: V, width: 3, bend: h.bend, progress: u, headSize: 12 });
        const la = g.at(h.start + h.dur - 0.1, 0.3, 'out');
        g.alpha(la, () => {
          const [mx] = g.pointAt(pts, 0.5);
          g.text('skip ' + h.k, mx, h.ly, { size: 18, font: 'mono', color: V, align: 'center' });
        });
      });
      // The result: c2 is main~5.
      const r = g.at(C2 + 5.3, 0.4, 'out');
      const rb = g.at(C2 + 5.3, 0.5, 'pulse');
      g.alpha(r, () => {
        g.circle(X(2), y, 31 + 4 * rb, { stroke: C.teal, lineWidth: 3 });
        g.chip('main~5', X(2), y + 100, { color: C.teal, size: 18, align: 'center' });
      });
    });
    g.alpha(g.vis(C2 + 4.0, skipEnd), () => {
      const w = g.chip('main~5 = skip 1 + skip 4', 110, 840, { color: V, size: 22 });
      g.text('O(log n) hops instead of walking back one changeset at a time', 110 + w + 26, 847, { size: 21, color: C.muted });
    });

    // Caption 3: blame on the tip needs history, which walks back from c7 to
    // the first ancestor that has it (c4), then derives c5, c6, c7.
    const tip = X(n - 1);
    const sy = y - 196;
    g.alpha(g.vis(C3 + 0.1), () => {
      g.chip('agit blame src/app.go', tip + 25, 400, { color: V, size: 22, align: 'right' });
    });
    const scan = g.at(C3 + 0.5, 1.2, 'inOut');
    g.alpha(g.vis(C3 + 0.4, C3 + 3.8), () => {
      g.line(tip, 420, tip, sy, { color: V, width: 3, dash: [6, 6] });
      const sxp = g.lerp(tip, X(4), scan);
      g.line(tip, sy, sxp, sy, { color: V, width: 3, dash: [8, 8] });
      g.circle(sxp, sy, 8, { fill: V });
      g.alpha(g.vis(C3 + 1.5), () => {
        g.text('c4 already has history', X(4), sy - 20, { size: 19, color: V, align: 'center' });
      });
    });
    g.alpha(g.vis(fill0 - 0.1), () => {
      const w = g.text('history derived oldest first: c5, c6, c7', 110, 847, { size: 21, color: C.muted });
      g.alpha(g.vis(histDerivedAt(7) + 0.5), () => {
        g.text(', then blame for each version of src/app.go', 110 + w, 847, { size: 21, color: V });
      });
    });
  },
});

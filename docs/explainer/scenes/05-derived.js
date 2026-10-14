// Scene 5: derived data — eager (tree, graph) and lazy (history, blame,
// copies) — plus the skip list behind main~N.
AGX.scene({
  id: 'derived',
  order: 5,
  title: 'Derived data',
  duration: 16,
  captions: [
    { at: 0, text: 'Everything else the server knows is derived data, computed from a changeset plus what was already derived for its parents.' },
    { at: 5.5, text: 'Root trees and the commit-graph index are derived eagerly, the moment a changeset arrives. Skip pointers turn main~5 into two hops.' },
    { at: 10.5, text: 'Path history, rename detection and blame are derived lazily, the first time someone asks, and then stored.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Derived data, computed from parents', { alpha: g.vis(0) });

    const n = 8;
    const x0 = 230, dx = 205, y = 650;
    const X = (i) => x0 + i * dx;
    const arrive = g.at(1.6, 1.0, 'out'); // the newest changeset slides in

    // Legend.
    g.alpha(g.vis(0.4), () => {
      g.chip('eager', 110, 312, { color: C.teal, size: 20 });
      g.text('computed when a changeset is created', 210, 319, { size: 21, color: C.muted });
      g.chip('lazy', 860, 312, { color: C.violet, size: 20, fill: 'rgba(0,0,0,0)' });
      g.text('computed on first request, then stored', 948, 319, { size: 21, color: C.muted });
    });

    // Chain line.
    g.alpha(g.vis(0.3), () => {
      g.line(X(0), y, X(n - 2), y, { color: C.line, width: 4 });
      g.line(X(n - 2), y, g.lerp(X(n - 2), X(n - 1), arrive), y, { color: C.line, width: 4 });
    });

    // Lazy history fill order: walk back from c7 to the first derived
    // ancestor (c4), then derive c5, c6, c7 oldest first.
    const histDerivedAt = (i) => {
      if (i <= 4) return -1; // already derived earlier
      return 12.6 + (i - 5) * 0.8;
    };

    for (let i = 0; i < n; i++) {
      const newest = i === n - 1;
      const xi = newest ? g.lerp(X(i) + 120, X(i), arrive) : X(i);
      const a = newest ? arrive : g.stagger(i, 0.3, 0.08, 0.4);
      g.alpha(a, () => {
        g.circle(xi, y, 22, { fill: newest ? g.rgba(C.amber, 0.25) : C.bg, stroke: C.amber, lineWidth: 4 });
        g.text('c' + i, xi, y + 58, { size: 20, font: 'mono', color: C.muted, align: 'center' });

        // Eager chips: tree and graph.
        const eagerA = newest ? g.at(2.7, 0.4, 'out') : 1;
        g.alpha(eagerA, () => {
          g.chip('tree', xi, y - 70, { color: C.teal, align: 'center', size: 17 });
          g.chip('graph', xi, y - 112, { color: C.teal, align: 'center', size: 17 });
        });
        // Lazy chip: history (and blame on the newest).
        const at = histDerivedAt(i);
        const filled = at < 0 ? true : t >= at;
        const pop = at < 0 ? 1 : g.at(at, 0.35, 'outBack');
        g.chip('history', xi, y - 154, {
          color: C.violet, align: 'center', size: 17,
          fill: filled ? g.rgba(C.violet, 0.22 * Math.min(1, pop)) : 'rgba(0,0,0,0)',
          stroke: filled ? g.rgba(C.violet, 0.8) : g.rgba(C.violet, 0.35),
          textColor: filled ? C.violet : g.rgba(C.violet, 0.45),
        });
      });
    }
    g.alpha(g.vis(2.8, 5.6), () => {
      g.text('new changeset: tree and graph computed on arrival', X(n - 1), y + 108, { size: 20, color: C.teal, align: 'right' });
    });

    // Skip pointers from the tip: 1, 2 and 4 first-parent steps back.
    const tip = X(n - 1);
    const skips = [[n - 2, 1], [n - 3, 2], [n - 5, 4]];
    skips.forEach(([j, k], i) => {
      const u = g.at(6.2 + i * 0.5, 0.8);
      g.alpha(1 - g.at(10.2, 0.5), () => {
        const pts = g.arrow(tip, y + 26, X(j), y + 26, { color: g.rgba(C.teal, 0.8), width: 3, bend: -(60 + i * 45), progress: u, headSize: 12 });
        if (u > 0.9) {
          const [mx, my] = g.pointAt(pts, 0.5);
          g.text('skip ' + k, mx, my + 34, { size: 18, font: 'mono', color: C.teal, align: 'center' });
        }
      });
    });
    g.alpha(g.vis(8.0, 10.2), () => {
      g.chip('main~5 = skip 4 + skip 1', 110, 840, { color: C.teal, size: 22 });
      g.text('O(log n) hops instead of walking back one changeset at a time', 480, 847, { size: 21, color: C.muted });
    });

    // Lazy request: blame on the tip walks back to a derived ancestor.
    g.alpha(g.vis(10.6), () => {
      g.chip('agit blame src/app.go', 110, 840, { color: C.violet, size: 22 });
    });
    const scan = g.at(11.0, 1.4, 'inOut');
    g.alpha(g.vis(11.0, 12.6), () => {
      const sxp = g.lerp(tip, X(4), scan);
      g.line(tip, y - 196, sxp, y - 196, { color: C.violet, width: 3, dash: [8, 8] });
      g.circle(sxp, y - 196, 8, { fill: C.violet });
      if (scan > 0.98) g.text('c4 already has it', X(4), y - 216, { size: 19, color: C.violet, align: 'center' });
    });
    g.alpha(g.vis(13.6), () => {
      g.text('derived oldest first: c5, c6, c7', 720, 847, { size: 21, color: C.muted });
    });
  },
});

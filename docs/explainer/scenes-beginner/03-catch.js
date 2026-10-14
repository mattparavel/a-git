// Beginner 3: the catch with copying everything.
AGX.scene({
  id: 'catch',
  order: 3,
  title: 'The catch',
  duration: 24,
  captions: [
    { at: 0, text: 'That full copy is handy when you work offline. But big projects have years of history, and copying all of it takes a while.' },
    { at: 9, text: "Today, many newcomers to a project aren't people at all. Build machines and AI coding helpers start from scratch many times a day." },
    { at: 17.6, text: 'Each one downloads the entire history, just to read a handful of files.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Copying everything, again and again', { alpha: g.vis(0) });

    // A big project history on the shared copy.
    const sx = 330, sy = 640;
    g.alpha(g.vis(0.3), () => {
      BK.stack(g, sx, sy, { scale: 1.6, layers: 11 });
      g.text('10 years of history', sx, sy + 90, { size: 26, weight: 600, color: C.ink, align: 'center' });
      g.text('shared copy', sx, sy + 126, { size: 22, color: C.muted, align: 'center' });
    });
    // A slow progress bar for a single full clone.
    g.alpha(g.vis(3.0, 8.6), () => {
      const bx = 820, by = 470, bw = 760;
      g.text('copying the full history…', bx, by - 22, { size: 24, color: C.muted });
      g.box(bx, by, bw, 26, { fill: C.panel, stroke: C.line, radius: 13 });
      const u = g.at(3.2, 5.4, 'linear');
      g.box(bx, by, Math.max(26, bw * u), 26, { fill: g.rgba(C.amber, 0.8), radius: 13 });
      g.text(Math.round(u * 100) + '%', bx + bw + 20, by + 22, { size: 24, font: 'mono', color: C.amber });
    });

    // Many machines, each copying everything.
    const helpers = [];
    for (let r = 0; r < 2; r++) for (let c = 0; c < 4; c++) helpers.push({ x: 860 + c * 230, y: 410 + r * 250 });
    const icons = ['agent', 'ci', 'agent', 'ci', 'ci', 'agent', 'agent', 'ci'];
    const names = ['AI helper', 'build', 'AI helper', 'test run', 'build', 'AI helper', 'AI helper', 'test run'];
    helpers.forEach((h, i) => {
      const appear = g.at(9.4 + i * 0.25, 0.5, 'out');
      g.alpha(appear, () => BK.helper(g, h.x, h.y, { icon: icons[i], name: names[i] }));
      const start = 11.0 + i * 0.45;
      const pts = g.curvePoints(sx + 110, sy - 80, h.x - 60, h.y, (i % 2 ? 1 : -1) * 30);
      const u = g.at(start, 1.6, 'inOut');
      if (u > 0 && u < 1) {
        g.path(pts, { color: g.rgba(C.amber, 0.3), width: 2, dash: [6, 8], progress: u });
        const [x, y] = g.pointAt(pts, u);
        BK.stack(g, x, y + 20, { scale: 0.42 });
      }
      if (t > start + 1.6) {
        g.alpha(g.at(start + 1.6, 0.3) * (1 - 0.7 * g.at(18.4, 1.0)), () => BK.stack(g, h.x + 70, h.y - 20, { scale: 0.42 }));
        // What it actually needed: a few files.
        g.alpha(g.at(18.6, 0.6), () => {
          g.box(h.x + 50, h.y - 40, 40, 12, { fill: C.mint, radius: 3 });
        });
      }
    });

    // The analogy.
    g.alpha(g.vis(19.4), () => {
      BK.bubble(g, 820, 862, 'like photocopying a whole library to read one chapter', { color: C.mint, size: 26 });
    });
  },
});

// Beginner 3: the catch with copying everything.
AGX.scene({
  id: 'catch',
  order: 3,
  title: 'The catch',
  duration: 25,
  captions: [
    { at: 0, text: 'That full copy is handy when you work offline. But big projects have years of history, and copying all of it takes a while.' },
    { at: 9.2, text: "Today, many newcomers aren't people. Machines that build and test code, and AI helpers, start fresh many times a day." },
    { at: 17.8, text: 'Often, each one downloads the entire history just to read a handful of files.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Copying everything, again and again', { alpha: g.vis(0) });

    // A big project history on the shared copy (the server from scene 2).
    const sx = 330, sy = 640;
    g.alpha(g.vis(0.3), () => {
      BK.stack(g, sx, sy, { scale: 1.6, layers: 11 });
      g.text('10 years of history', sx, sy + 90, { size: 26, weight: 600, color: C.ink, align: 'center' });
      g.text('server (shared copy)', sx, sy + 126, { size: 22, color: C.muted, align: 'center' });
    });

    // You: the full copy lets you work offline, but getting it is slow.
    const you = { x: 1640, y: 470 };
    g.alpha(g.vis(0.6, 8.6), () => {
      BK.person(g, you.x, you.y, { name: 'You', color: C.blue });
      g.alpha(g.at(1.4, 0.4), () => g.chip('works offline', you.x, you.y + 120, { color: C.mint, size: 22, font: 'body', align: 'center' }));
    });
    g.alpha(g.vis(3.0, 8.6), () => {
      const bx = 560, by = you.y - 13, bw = 820;
      g.text('copying the full history…', bx, by - 22, { size: 24, color: C.muted });
      g.box(bx, by, bw, 26, { fill: C.panel, stroke: C.line, radius: 13 });
      const u = g.at(3.2, 5.4, 'linear');
      g.box(bx, by, Math.max(26, bw * u), 26, { fill: g.rgba(C.amber, 0.8), radius: 13 });
      g.text(Math.round(u * 100) + '%', bx + bw + 20, by + 22, { size: 24, font: 'mono', color: C.amber });
    });

    // They start over many times a day.
    g.alpha(g.vis(14.6, 17.8), () => g.chip('many times a day', 1190, 292, { color: C.blue, size: 24, font: 'body', align: 'center' }));

    // Many machines, each copying everything.
    const helpers = [];
    for (let r = 0; r < 2; r++) for (let c = 0; c < 4; c++) helpers.push({ x: 890 + c * 240, y: 390 + r * 230 });
    const icons = ['agent', 'ci', 'agent', 'ci', 'ci', 'agent', 'agent', 'ci'];
    const names = icons.map((ic) => (ic === 'ci' ? 'build & test' : 'AI helper'));
    const dim = 1 - 0.55 * g.at(18.2, 0.8);
    helpers.forEach((h, i) => {
      const appear = g.at(9.3 + i * 0.22, 0.5, 'out');
      g.alpha(appear, () => BK.helper(g, h.x, h.y, { icon: icons[i], name: names[i] }));
      // Each one's copy lands just left of it.
      const cx = h.x - 110, cy = h.y + 26;
      const start = 11.2 + i * 0.45;
      const pts = g.curvePoints(sx + 100, sy - 120, cx, cy - 20, (i % 2 ? 1 : -1) * 30);
      const u = g.at(start, 1.6, 'inOut');
      if (u > 0 && u < 1) {
        g.path(pts, { color: g.rgba(C.amber, 0.3), width: 2, dash: [6, 8], progress: u });
        const [x, y] = g.pointAt(pts, u);
        BK.stack(g, x, y + 20, { scale: 0.55 });
      }
      if (u >= 1) {
        g.alpha(dim, () => BK.stack(g, cx, cy, { scale: 0.55 }));
        // What it actually needed: a few files, in front of the unused copy.
        g.alpha(g.at(18.8 + i * 0.08, 0.4), () => {
          g.box(cx + 4, cy - 34, 30, 38, { fill: C.bg, radius: 4 });
          g.icon('file', cx + 19, cy - 15, 40, { color: C.mint, stroke: 8 });
        });
      }
    });

    // One plain legend for the two marks.
    const ly = 780, lx = 1190;
    const la = 'downloaded: everything', lb = 'needed: a few files';
    const wa = g.measure(la, { size: 22 }), wb = g.measure(lb, { size: 22 });
    const total = 58 + wa + 60 + 34 + wb;
    const x0 = lx - total / 2;
    g.alpha(g.vis(18.6), () => {
      g.alpha(0.45, () => BK.stack(g, x0 + 22, ly + 8, { scale: 0.4 }));
      g.text(la, x0 + 58, ly, { size: 22, color: C.muted });
    });
    g.alpha(g.vis(19.4), () => {
      const xb = x0 + 58 + wa + 60;
      g.icon('file', xb + 10, ly - 8, 30, { color: C.mint, stroke: 9 });
      g.text(lb, xb + 34, ly, { size: 22, color: C.mint });
    });

    // The analogy.
    g.alpha(g.vis(20.8), () => {
      BK.bubble(g, lx, 870, 'like photocopying a whole library to read one chapter', { color: C.ink, size: 26, align: 'center' });
    });
  },
});

// Beginner 8: copies kept close to you make things fast.
AGX.scene({
  id: 'copies',
  order: 8,
  title: 'Copies close to you',
  duration: 24,
  captions: [
    { at: 0, text: 'To stay fast, a-git keeps copies close to you: one on your own machine, one nearby in your region, and the main server.' },
    { at: 8.6, text: 'The first time you ask for something, it comes from the server and is copied along the way. Next time, it comes straight from your machine.' },
    { at: 18.2, text: 'Saved versions never change, so a copy can never be out of date.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Copies close to you keep it fast', { alpha: g.vis(0) });

    const y = 540;
    const stops = [
      { x: 250, label: 'You', kind: 'person' },
      { x: 700, label: 'your machine', sub: 'a small helper app', icon: 'laptop', color: C.blue },
      { x: 1150, label: 'your region', sub: 'a shared nearby copy', icon: 'db', color: '#6FBFD8' },
      { x: 1600, label: 'main server', sub: 'the full history', icon: 'server', color: C.teal },
    ];
    stops.forEach((s, i) => {
      g.alpha(g.stagger(i, 0.3, 0.6, 0.5), () => {
        if (s.kind === 'person') BK.person(g, s.x, y, { name: 'You', color: C.blue });
        else g.node(s.x, y, { icon: s.icon, label: s.label, sub: s.sub, color: s.color, w: 280, h: 210, labelFont: 'body', labelSize: 26, subSize: 19 });
      });
      if (i > 0) {
        const x1 = stops[i - 1].x + (i === 1 ? 70 : 140), x2 = s.x - 140;
        g.alpha(g.vis(0.6 + i * 0.6), () => g.line(x1, y, x2, y, { color: C.line, width: 3 }));
      }
    });

    // First request: all the way to the server and back, leaving copies.
    const lane = (a, b, off) => [[stops[a].x + (a === 0 ? 70 : 140) * Math.sign(b - a), y + off], [stops[b].x - 140 * Math.sign(b - a), y + off]];
    g.packet(lane(0, 1, -36), g.at(9.0, 0.8), { color: C.blue, label: 'app.go, please', size: 20, dy: -30 });
    g.packet(lane(1, 2, -36), g.at(9.8, 0.8), { color: C.blue });
    g.packet(lane(2, 3, -36), g.at(10.6, 0.8), { color: C.blue });
    g.packet(lane(3, 2, 36), g.at(11.6, 0.8), { color: C.amber });
    g.packet(lane(2, 1, 36), g.at(12.4, 0.8), { color: C.amber });
    g.packet(lane(1, 0, 36), g.at(13.2, 0.8), { color: C.amber });
    const copyAt = { 2: 12.4, 1: 13.2 };
    [1, 2].forEach((i) => {
      g.alpha(g.vis(copyAt[i]), () => {
        g.icon('file', stops[i].x, y + 150, 34, { color: C.amber, stroke: 9 });
        g.text('copy of app.go', stops[i].x, y + 196, { size: 20, color: C.amber, align: 'center' });
      });
    });
    g.alpha(g.vis(9.0), () => {
      g.icon('file', stops[3].x, y + 150, 34, { color: C.amber, stroke: 9 });
      g.text('app.go', stops[3].x, y + 196, { size: 20, color: C.amber, align: 'center' });
    });

    // Second request: answered right away by your own machine.
    g.packet(lane(0, 1, -36), g.at(15.0, 0.7), { color: C.blue, label: 'app.go again', size: 20, dy: -30 });
    g.packet(lane(1, 0, 36), g.at(15.8, 0.6), { color: C.mint });
    g.alpha(g.vis(15.8), () => g.chip('instant: from your machine', stops[1].x, y - 160, { color: C.mint, size: 24, align: 'center' }));

    // Why copies are safe.
    g.alpha(g.vis(18.4), () => {
      g.icon('lock', 760, 850, 30, { color: C.mint, stroke: 8 });
      g.text('a saved version never changes, so every copy stays correct', 796, 862, { size: 26, color: C.mint });
    });
  },
});

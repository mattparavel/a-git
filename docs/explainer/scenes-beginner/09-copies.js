// Beginner 9: copies kept close to you make things fast.
AGX.scene({
  id: 'copies',
  order: 9,
  title: 'Copies close to you',
  duration: 27.4,
  captions: [
    { at: 0, text: 'To stay fast, a-git can keep copies nearby: one on your computer and one shared in your region.' },
    { at: 7.4, text: 'The first time, the file comes from the main server and is copied along the way. Next time, your computer has it.' },
    { at: 16.0, text: 'A saved version never changes, so a copy of it is never wrong.' },
    { at: 20.8, text: 'But “what’s new on main?” always goes to the main server, so you see the latest.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Copies close to you keep it fast', { alpha: g.vis(0) });

    const y = 575;
    const REGION = '#6FBFD8';
    const stops = [
      { x: 290, at: 0.3 },
      { x: 720, at: 3.0, label: 'helper app', sub: 'optional, keeps copies', icon: 'laptop', color: C.blue },
      { x: 1190, at: 4.6, label: 'your region', sub: 'optional, shared nearby', icon: 'db', color: REGION },
      { x: 1620, at: 0.9, label: 'main server', sub: 'the full history', icon: 'server', color: C.teal },
    ];
    const NW = 280, NH = 210;

    // Your computer holds both you and the helper app.
    const bx = 130, by = 330, bw = 800, bh = 490;
    g.alpha(g.vis(0.3), () => {
      g.box(bx, by, bw, bh, { fill: g.rgba(C.blue, 0.05), stroke: g.rgba(C.blue, 0.5), radius: 20 });
      g.icon('laptop', bx + 50, by + 48, 46, { color: C.blue });
      g.text('your computer', bx + 92, by + 60, { size: 28, font: 'display', weight: 700, color: C.ink });
    });

    // One road from you to the main server; the copies sit along it.
    g.alpha(g.vis(0.9), () => g.line(stops[0].x + 70, y, stops[3].x - NW / 2, y, { color: C.line, width: 3 }));

    // Who is busy right now (a soft highlight).
    const busy = (i) => {
      if (i === 1) return g.vis(stops[1].at, stops[1].at + 1.2) + g.vis(14.4, 15.6);
      if (i === 2) return g.vis(stops[2].at, stops[2].at + 1.2);
      if (i === 3) return g.vis(10.3, 11.0) + g.vis(23.2, 24.2);
      return 0;
    };
    stops.forEach((s, i) => {
      g.alpha(g.vis(s.at), () => {
        if (i === 0) BK.person(g, s.x, y, { name: 'You', color: C.blue });
        else g.node(s.x, y, {
          icon: s.icon, label: s.label, sub: s.sub, color: i === 1 && t > 14.4 && t < 16.0 ? C.mint : s.color,
          w: NW, h: NH, labelFont: 'body', labelSize: 26, subSize: 21, active: busy(i) > 0.5,
        });
      });
    });

    // Packets travel in two lanes: requests above the road, answers below.
    const edge = (i, dir) => stops[i].x + dir * (i === 0 ? 70 : NW / 2);
    const lane = (a, b, off) => {
      const dir = Math.sign(b - a);
      return [[edge(a, dir), y + off], [edge(b, -dir), y + off]];
    };
    const hop = (a, b, start, color, dur) => g.packet(lane(a, b, a < b ? -36 : 36), g.at(start, dur || 0.7), { color, r: 10 });

    // What you ask for, in your own words.
    const ask = (text, from, to) => g.alpha(g.vis(from, to), () => BK.bubble(g, stops[0].x - 40, y - 72, text, { color: C.blue, size: 22 }));

    // Files are dimmed once the story moves on to "what's new".
    const fileA = 1 - 0.55 * g.at(20.8, 0.6);

    // First request: all the way to the main server and back, leaving copies.
    // Ends early enough to be fully gone (0.4 s fade) before the next ask appears at 13.4.
    ask('app.go, please', 7.8, 12.9);
    hop(0, 1, 8.2, C.blue);
    hop(1, 2, 8.9, C.blue);
    hop(2, 3, 9.6, C.blue);
    hop(3, 2, 10.6, C.amber);
    hop(2, 1, 11.3, C.amber);
    hop(1, 0, 12.0, C.amber, 0.6);
    const fileAt = { 1: 12.0, 2: 11.3, 3: 7.6 };
    [1, 2, 3].forEach((i) => {
      g.alpha(g.vis(fileAt[i]) * fileA, () => {
        g.icon('file', stops[i].x, y + 152, 34, { color: C.amber, stroke: 9 });
        g.text(i === 3 ? 'app.go' : 'copy of app.go', stops[i].x, y + 199, { size: 21, color: C.amber, align: 'center' });
        // Saved versions are locked: they never change.
        g.alpha(g.stagger(i, 16.2, 0.3, 0.4), () => g.icon('lock', stops[i].x + 38, y + 150, 30, { color: C.mint, stroke: 7 }));
      });
    });
    // The copies match the original.
    g.alpha(g.vis(17.4) * fileA, () => {
      [1010, 1420].forEach((x) => g.text('=', x, y + 168, { size: 44, weight: 600, color: C.mint, align: 'center' }));
    });
    g.alpha(g.vis(17.4, 20.8), () => g.text('locked versions, so copies always match', 1420, y + 280, { size: 24, color: C.mint, align: 'center' }));

    // Second request for the same version: answered by your own computer.
    ask('same version of app.go', 13.4, 16.0);
    hop(0, 1, 13.8, C.blue, 0.6);
    hop(1, 0, 14.5, C.mint, 0.6);
    g.alpha(g.vis(14.5, 16.0), () => g.chip('already here: no download', stops[1].x, by + 108, { color: C.mint, size: 22, align: 'center', font: 'body', weight: 600 }));

    // "What's new on main?" is always asked of the main server.
    ask('what’s new on main?', 21.0, 27.6);
    hop(0, 1, 21.4, C.blue, 0.6);
    hop(1, 2, 22.0, C.blue, 0.6);
    hop(2, 3, 22.6, C.blue, 0.6);
    hop(3, 2, 23.6, C.teal, 0.6);
    hop(2, 1, 24.2, C.teal, 0.6);
    hop(1, 0, 24.8, C.teal, 0.6);
    g.alpha(g.vis(23.2), () => g.chip('main: the newest version', stops[3].x, by + 108, { color: C.teal, size: 22, align: 'center', font: 'body', weight: 600 }));
    g.alpha(g.vis(25.3), () => {
      g.icon('check', stops[0].x - 52, y + 150, 26, { color: C.teal, stroke: 8 });
      g.text('you see the latest', stops[0].x - 30, y + 158, { size: 22, color: C.teal });
    });
    g.alpha(g.vis(21.8), () => {
      [1, 2].forEach((i) => g.text('passes it on', stops[i].x, y - NH / 2 - 18, { size: 20, color: C.muted, align: 'center' }));
    });
  },
});

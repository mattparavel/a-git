// Beginner 6: landing on main, and what happens when two people land at once.
AGX.scene({
  id: 'land',
  order: 6,
  title: 'Landing on main',
  duration: 26,
  captions: [
    { at: 0, text: 'When your work is ready, you land it on main, the shared branch everyone builds on.' },
    { at: 7, text: 'If two people land at the same moment, the server puts one first, then replays the other on top automatically. Nobody waits in line.' },
    { at: 17.4, text: 'Only when both changed the same part of the same file does a person need to step in and decide.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Landing your work on main', { alpha: g.vis(0) });

    // The main line.
    const y = 560;
    const base = [380, 540, 700];
    g.alpha(g.vis(0.3), () => {
      g.line(260, y, 1720, y, { color: C.lineSoft, width: 4, dash: [2, 12] });
      g.line(260, y, base[2], y, { color: C.teal, width: 6 });
      base.forEach((x) => g.circle(x, y, 18, { fill: C.bg, stroke: C.teal, lineWidth: 5 }));
      g.text('main', 260, y - 40, { size: 28, font: 'mono', weight: 600, color: C.teal });
    });

    // Robin and Sam, each with a change ready.
    const robin = { x: 330, y: 340 }, sam = { x: 330, y: 790 };
    g.alpha(g.vis(0.8), () => {
      BK.person(g, robin.x, robin.y, { name: 'Robin', color: C.blue, size: 0.8 });
      BK.person(g, sam.x, sam.y, { name: 'Sam', color: C.violet, size: 0.8 });
    });
    const card = (x, y, label, col, a) => g.alpha(a, () => {
      g.box(x - 90, y - 32, 180, 64, { fill: g.rgba(col, 0.18), stroke: g.rgba(col, 0.9), radius: 12, lineWidth: 2 });
      g.text(label, x, y + 9, { size: 24, weight: 600, color: C.ink, align: 'center' });
    });

    // Robin lands first: the change joins main at x=860.
    const rMove = g.at(8.0, 1.6, 'inOut');
    const rx = g.lerp(520, 860, rMove), ry = g.lerp(robin.y, y - 90, rMove);
    card(rx, ry, "Robin's change", C.blue, g.vis(1.6));
    // Sam's change heads for main at the same moment, waits a beat, then is
    // replayed on top of Robin's.
    const sMove1 = g.at(8.0, 1.6, 'inOut');
    const sMove2 = g.at(11.0, 1.6, 'inOut');
    const sx = g.lerp(g.lerp(520, 860, sMove1), 1060, sMove2);
    const sy = g.lerp(g.lerp(sam.y, y + 90, sMove1), y - 90, sMove2);
    card(sx, sy, "Sam's change", C.violet, g.vis(1.6));
    g.alpha(g.vis(7.4, 8.6), () => g.chip('both land at once', 960, 760, { color: C.amber, size: 24, align: 'center' }));

    // Main grows: Robin's change, then Sam's on top.
    const r1 = g.at(9.6, 0.5, 'outBack'), s1 = g.at(12.6, 0.5, 'outBack');
    if (r1 > 0) {
      g.line(base[2], y, g.lerp(base[2], 860, Math.min(1, r1)), y, { color: C.teal, width: 6 });
      g.circle(860, y, 18 * Math.min(1, r1), { fill: g.rgba(C.blue, 0.35), stroke: C.blue, lineWidth: 5 });
    }
    if (s1 > 0) {
      g.line(860, y, g.lerp(860, 1060, Math.min(1, s1)), y, { color: C.teal, width: 6 });
      g.circle(1060, y, 18 * Math.min(1, s1), { fill: g.rgba(C.violet, 0.35), stroke: C.violet, lineWidth: 5 });
    }
    g.alpha(g.vis(9.8, 16.8), () => g.text('1st', 860, y + 56, { size: 24, weight: 600, color: C.blue, align: 'center' }));
    g.alpha(g.vis(12.8, 16.8), () => {
      g.text('2nd, replayed on top', 1060, y + 56, { size: 24, weight: 600, color: C.violet, align: 'center' });
      g.chip('no waiting in line', 1300, 360, { color: C.mint, size: 24 });
    });

    // When both changed the same lines: a person decides.
    g.alpha(g.vis(17.6), () => {
      const bx = 1180, by = 680;
      g.box(bx, by, 560, 190, { fill: C.panel, stroke: g.rgba(C.amber, 0.7), radius: 16 });
      g.text('Robin:  price = 10', bx + 30, by + 56, { size: 24, font: 'mono', color: C.blue });
      g.text('Sam:    price = 12', bx + 30, by + 100, { size: 24, font: 'mono', color: C.violet });
      g.text('same line, so someone decides', bx + 30, by + 154, { size: 23, weight: 600, color: C.amber });
    });
  },
});

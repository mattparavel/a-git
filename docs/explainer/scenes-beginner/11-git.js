// Beginner 11: works alongside Git — bring history in, send it back out.
AGX.scene({
  id: 'git',
  order: 11,
  title: 'Works with Git',
  duration: 16,
  captions: [
    { at: 0, text: "Already using Git? You can bring your project's whole history into a-git." },
    { at: 6.6, text: 'And you can copy it back out to a Git repository at any time, so tools that expect Git keep working.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Works alongside Git', { alpha: g.vis(0) });

    const y = 470;
    const L = { x: 330 }, M = { x: 960 }, R = { x: 1590 };
    g.alpha(g.vis(0.3), () => g.node(L.x, y, { icon: 'git', label: 'your Git project', color: C.muted, w: 300, h: 210, labelFont: 'body', labelSize: 24 }));
    g.alpha(g.vis(0.6), () => g.node(M.x, y, { icon: 'server', label: 'a-git', color: C.teal, w: 300, h: 210, labelFont: 'body', labelSize: 26 }));
    g.alpha(g.vis(6.8), () => g.node(R.x, y, { icon: 'git', label: 'a Git copy', color: C.muted, w: 300, h: 210, labelFont: 'body', labelSize: 24 }));

    g.arrow(L.x + 160, y, M.x - 160, y, { color: C.amber, width: 4, progress: g.at(1.4, 1.2), headSize: 18 });
    g.alpha(g.vis(2.0), () => g.chip('agit import-git', (L.x + M.x) / 2, y - 44, { color: C.amber, size: 24, align: 'center' }));
    g.arrow(M.x + 160, y, R.x - 160, y, { color: C.amber, width: 4, progress: g.at(7.4, 1.2), headSize: 18 });
    g.alpha(g.vis(8.0), () => g.chip('agit export-git', (M.x + R.x) / 2, y - 44, { color: C.amber, size: 24, align: 'center' }));

    // The same history on both sides.
    const history = (cx, at, col) => g.alpha(g.vis(at), () => {
      const xs = [-160, -80, 0, 80, 160];
      g.line(cx + xs[0], 760, cx + xs[4], 760, { color: col, width: 4 });
      xs.forEach((dx, i) => g.circle(cx + dx, 760, 14, { fill: C.bg, stroke: col, lineWidth: 4 }));
      g.text('5 years of commits', cx, 816, { size: 22, color: C.muted, align: 'center' });
    });
    history(L.x, 1.0, C.amber);
    history(M.x, 3.0, C.teal);
    history(R.x, 9.0, C.amber);
    g.alpha(g.vis(10.4), () => {
      g.icon('check', R.x + 200, 760, 40, { color: C.mint, stroke: 6 });
      g.text('same history', R.x, 866, { size: 24, weight: 600, color: C.mint, align: 'center' });
    });
  },
});

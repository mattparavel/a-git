// Beginner 11: works alongside Git — bring history in, copy it back out, and
// refresh the copy later (only new commits travel).
AGX.scene({
  id: 'git',
  order: 11,
  title: 'Works with Git',
  duration: 19,
  captions: [
    { at: 0, text: "Already using Git? You can bring your project's whole history into a-git." },
    { at: 6.4, text: 'You can also copy it back out to a Git repository, so tools that only know Git can still read it.' },
    { at: 14.0, text: 'Run it again later, and only the new commits are copied over.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Works alongside Git', { alpha: g.vis(0) });

    const y = 450;
    const L = { x: 330 }, M = { x: 960 }, R = { x: 1590 };
    const RY = 690, STEP = 72;
    // During the last caption the old Git project steps back.
    const dimL = 1 - 0.6 * g.at(14.0, 0.6);

    // --- the three places ---------------------------------------------
    g.alpha(g.vis(0.3) * dimL, () => g.node(L.x, y, { icon: 'git', label: 'your Git project', color: C.muted, w: 300, h: 200, labelFont: 'body', labelSize: 26 }));
    g.alpha(g.vis(0.6), () => g.node(M.x, y, { icon: 'server', label: 'a-git', color: C.teal, w: 300, h: 200, labelFont: 'body', labelSize: 28 }));
    g.alpha(g.vis(6.6), () => g.node(R.x, y, { icon: 'git', label: 'a Git copy', color: C.muted, w: 300, h: 200, labelFont: 'body', labelSize: 26 }));

    // --- the two copies, with commits travelling along the arrows -------
    const inPts = [[L.x + 160, y], [M.x - 160, y]];
    const outPts = [[M.x + 160, y], [R.x - 160, y]];
    g.alpha(dimL, () => {
      g.arrow(inPts[0][0], y, inPts[1][0], y, { color: C.amber, width: 4, progress: g.at(1.6, 1.0), headSize: 18 });
      g.alpha(g.vis(2.0), () => g.chip('agit import-git', (L.x + M.x) / 2, y - 44, { color: C.amber, size: 24, align: 'center' }));
    });
    g.arrow(outPts[0][0], y, outPts[1][0], y, { color: C.amber, width: 4, progress: g.at(7.2, 1.0), headSize: 18 });
    g.alpha(g.vis(7.6), () => g.chip('agit export-git', (M.x + R.x) / 2, y - 44, { color: C.amber, size: 24, align: 'center' }));

    // Oldest commit first: each dot travels, then lands in the next row.
    const IN0 = 2.2, OUT0 = 7.8, GAP = 0.35, TRIP = 0.8;
    for (let i = 0; i < 5; i++) {
      g.packet(inPts, g.at(IN0 + i * GAP, TRIP, 'inOut'), { color: C.amber, r: 10 });
      g.packet(outPts, g.at(OUT0 + i * GAP, TRIP, 'inOut'), { color: C.amber, r: 10 });
    }
    // Later: one new commit appears in a-git and is copied on its own.
    const NEW_AT = 14.5, NEW_OUT = 15.6;
    g.packet(outPts, g.at(NEW_OUT, 1.0, 'inOut'), { color: C.mint, r: 11 });

    // A row of commits. alphaOf(i) fades each dot in; grow (0..1) slides
    // the row left to make room for one new commit on the right.
    const row = (cx, col, alphaOf, grow) => {
      const off = -STEP / 2 * grow;
      const xs = [0, 1, 2, 3, 4].map((i) => cx + (i - 2) * STEP + off);
      const nx = cx + 3 * STEP + off;
      for (let i = 1; i < 5; i++) g.line(xs[i - 1], RY, xs[i], RY, { color: col, width: 4, alpha: alphaOf(i) });
      if (grow > 0) g.line(xs[4], RY, nx, RY, { color: col, width: 4, alpha: grow });
      xs.forEach((x, i) => g.circle(x, RY, 14, { fill: C.bg, stroke: col, lineWidth: 4, alpha: alphaOf(i) }));
      if (grow > 0) {
        g.circle(nx, RY, 14, { fill: g.rgba(C.mint, 0.25), stroke: C.mint, lineWidth: 4, alpha: grow });
        g.text('new', nx, RY - 30, { size: 20, weight: 600, color: C.mint, align: 'center', alpha: grow });
      }
    };
    const label = (cx, at) => g.text('10 years of history', cx, RY + 52, { size: 22, color: C.muted, align: 'center', alpha: g.vis(at) });

    g.alpha(dimL, () => {
      row(L.x, C.amber, (i) => g.vis(0.8 + i * 0.08), 0);
      label(L.x, 1.0);
    });
    row(M.x, C.teal, (i) => g.at(IN0 + TRIP + i * GAP, 0.25), g.at(NEW_AT, 0.6));
    label(M.x, 4.4);
    row(R.x, C.amber, (i) => g.at(OUT0 + TRIP + i * GAP, 0.25), g.at(NEW_OUT + 1.0, 0.5));
    label(R.x, 10.0);

    // Same history on the Git side. It dims while a-git has a commit the
    // copy has not received yet, and lights up again once it arrives.
    const same = g.vis(10.6) * (1 - 0.7 * g.at(NEW_AT, 0.5) + 0.7 * g.at(NEW_OUT + 1.2, 0.5));
    g.alpha(same, () => {
      const tw = g.measure('same history', { size: 26, weight: 600 });
      g.icon('check', R.x - tw / 2 - 26, RY + 104, 32, { color: C.mint, stroke: 7 });
      g.text('same history', R.x + 14, RY + 114, { size: 26, weight: 600, color: C.mint, align: 'center' });
    });

    // Tools that only know Git read the copy.
    g.alpha(g.vis(11.2), () => {
      const bx = R.x, by = 290;
      g.icon('eye', bx - 92, by - 8, 36, { color: C.violet, stroke: 7 });
      g.text('Git tools read it', bx + 22, by, { size: 24, weight: 600, color: C.violet, align: 'center' });
      g.arrow(bx, y - 104, bx, by + 20, { color: C.violet, width: 3, headSize: 12, progress: g.at(11.4, 0.5) });
    });
  },
});

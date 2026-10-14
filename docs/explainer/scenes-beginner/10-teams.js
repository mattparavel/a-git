// Beginner 10: folder permissions and reviews that notice what changed.
AGX.scene({
  id: 'teams',
  order: 10,
  title: 'Built for teams',
  duration: 27,
  captions: [
    { at: 0, text: 'Teams can protect folders. Only the payments team can change the payments code, and secret files stay hidden from anyone without access.' },
    { at: 10.2, text: 'Reviews notice what changed. Fix a typo in a code comment, a note for people, and the approval still counts.' },
    { at: 18.6, text: 'Change what the code does, and the old approval no longer counts. The reviewer looks again.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Built for teams', { alpha: g.vis(0) });

    // Part 1: protected folders. The card slides away when the review
    // starts, so attention moves to the review.
    const out = g.at(9.9, 0.6, 'inOut');
    const lx = 310 - 260 * out, ly = 300, lw = 1300;
    g.alpha(1 - out, () => {
      g.alpha(g.vis(0.3), () => g.card(lx, ly, lw, 470, { title: 'who can do what', titleSize: 22, color: C.teal }));
      const rows = [
        ['app/', 'everyone can edit', C.mint, false, 0.8],
        ['payments/', 'only the payments team can edit', C.amber, true, 2.8],
        ['secrets/', 'hidden from anyone without access', C.coral, true, 5.4],
      ];
      rows.forEach(([name, note, col, locked, at], i) => {
        const y = ly + 140 + i * 110;
        g.alpha(g.vis(at), () => {
          BK.folder(g, lx + 60, y, name, { color: col, size: 30 });
          if (locked) g.icon('lock', lx + 94 + g.measure(name, { size: 30, font: 'mono' }) + 30, y - 10, 30, { color: col, stroke: 8 });
          g.text(note, lx + 460, y, { size: 28, color: C.ink });
        });
      });
      g.alpha(g.vis(7.2), () => g.text('admins keep access everywhere', lx + 60, ly + 430, { size: 22, color: C.muted }));
    });

    // Part 2: a review across three versions of the same change.
    const rx = 110, rw = 1060, ry = 290;
    g.alpha(g.vis(10.5), () => g.card(rx, ry, rw, 600, { title: 'review', titleSize: 22, color: C.blue }));
    const stale = g.at(20.0, 0.6);
    const versions = [
      { at: 11.0, label: 'version 1', diff: ['+ total = price * 2'], stamp: ['approved by Sam', C.mint] },
      { at: 13.6, label: 'version 2', diff: ['- // retrys twice', '+ // retries twice'], stamp: ['approval still counts', C.mint], note: 'only a code comment changed' },
      { at: 19.0, label: 'version 3', diff: ['- total = price * 2', '+ total = price * 3'], stamp: ['approval no longer counts', C.coral], note: 'what the code does changed' },
    ];
    const stampAt = (v) => v.at + 1.0;
    const stampY = (i, v) => ry + 96 + i * 170 + (v.diff.length > 1 ? 52 : 34);
    versions.forEach((v, i) => {
      const y = ry + 96 + i * 170;
      g.alpha(g.vis(v.at), () => {
        g.text(v.label, rx + 34, y, { size: 26, weight: 600, color: C.ink });
        if (v.note) g.text(v.note, rx + 210, y, { size: 24, color: C.muted });
        v.diff.forEach((d, j) => {
          const col = d[0] === '+' ? C.mint : C.coral;
          g.text(d, rx + 34, y + 46 + j * 38, { size: 26, font: 'mono', color: col });
        });
      });
      // Once the code itself changes, the earlier "counts" stamps fade back.
      const dim = i < 2 ? 1 - 0.55 * stale : 1;
      g.alpha(g.vis(stampAt(v)) * dim, () => {
        g.chip(v.stamp[0], rx + rw - 30, stampY(i, v), { color: v.stamp[1], size: 24, font: 'body', align: 'right' });
      });
    });

    // Sam, the reviewer, beside the review.
    const sx = 1480, sy = 560;
    g.alpha(g.vis(10.7), () => {
      BK.person(g, sx, sy, { name: 'Sam', color: C.violet });
      g.text('reviewer', sx, sy + 56 + 70, { size: 22, color: C.muted, align: 'center' });
    });
    // Sam's approval goes to version 1.
    g.alpha(1 - 0.55 * stale, () => {
      g.arrow(sx - 66, sy - 30, rx + rw + 4, stampY(0, versions[0]), {
        color: g.rgba(C.mint, 0.8), width: 3, dash: [8, 8], progress: g.at(stampAt(versions[0]), 0.7), headSize: 14,
      });
    });
    // After the logic change, Sam takes another look.
    g.alpha(g.vis(21.2), () => BK.bubble(g, sx - 34, sy - 86, 'let me look again', { color: C.violet, size: 26 }));
  },
});

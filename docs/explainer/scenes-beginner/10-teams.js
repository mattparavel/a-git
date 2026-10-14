// Beginner 10: folder permissions and reviews that notice what changed.
AGX.scene({
  id: 'teams',
  order: 10,
  title: 'Built for teams',
  duration: 26,
  captions: [
    { at: 0, text: 'Teams can protect folders. Only the payments team can change the payments code, and secret files stay hidden from everyone else.' },
    { at: 9.6, text: "Reviews notice what changed. Fix a typo in a comment, and your teammate's approval still counts." },
    { at: 17.6, text: 'Change what the code actually does, and reviewers are asked to take another look.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Built for teams', { alpha: g.vis(0) });

    // Left: protected folders.
    const lx = 110, ly = 300;
    g.alpha(g.vis(0.3), () => g.card(lx, ly, 760, 420, { title: 'who can do what', color: C.teal }));
    const rows = [
      ['app/', 'everyone can edit', C.mint, false, 0.8],
      ['payments/', 'only the payments team can edit', C.amber, true, 2.6],
      ['secrets/', 'hidden from everyone else', C.coral, true, 5.2],
    ];
    rows.forEach(([name, note, col, locked, at], i) => {
      const y = ly + 120 + i * 96;
      g.alpha(g.vis(at), () => {
        BK.folder(g, lx + 50, y, name, { color: col, size: 28 });
        if (locked) g.icon('lock', lx + 80 + g.measure(name, { size: 28, font: 'mono' }) + 36, y - 10, 28, { color: col, stroke: 8 });
        g.text(note, lx + 50, y + 40, { size: 22, color: C.muted });
      });
    });

    // Right: a review across three versions of the same change.
    const rx = 960;
    g.alpha(g.vis(9.6), () => g.card(rx, 300, 850, 590, { title: 'review', color: C.blue }));
    const versions = [
      { at: 10.0, label: 'version 1', diff: ['+ total = price * 2'], stamp: ['approved by Sam', C.mint] },
      { at: 12.6, label: 'version 2', diff: ['- // retrys twice', '+ // retries twice'], stamp: ['approval still counts', C.mint], note: 'only a comment changed' },
      { at: 18.0, label: 'version 3', diff: ['- total = price * 2', '+ total = price * 3'], stamp: ['please look again', C.coral], note: 'what the code does changed' },
    ];
    versions.forEach((v, i) => {
      const y = 390 + i * 168;
      g.alpha(g.vis(v.at), () => {
        g.text(v.label, rx + 30, y, { size: 24, weight: 600, color: C.ink });
        if (v.note) g.text(v.note, rx + 200, y, { size: 21, color: C.muted });
        v.diff.forEach((d, j) => {
          const col = d[0] === '+' ? C.mint : C.coral;
          g.text(d, rx + 30, y + 44 + j * 36, { size: 24, font: 'mono', color: col });
        });
      });
      g.alpha(g.vis(v.at + 1.0), () => {
        g.chip(v.stamp[0], rx + 820, y + 44, { color: v.stamp[1], size: 22, align: 'right' });
      });
    });
    // Sam reviewing.
    g.alpha(g.vis(10.4), () => BK.person(g, 760, 800, { name: 'Sam', color: C.violet, size: 0.7, nameSize: 22 }));
  },
});

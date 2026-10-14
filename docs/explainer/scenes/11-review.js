// Scene 11: reviews with owners and semantic approval carry-over.
AGX.scene({
  id: 'review',
  order: 11,
  title: 'Semantic code review',
  duration: 18,
  captions: [
    { at: 0, text: 'Reviews use semantic rules. A REVIEWERS file names who owns each directory.' },
    { at: 6, text: 'When a new revision changes only comments or whitespace, approvals carry over. A language-aware classifier decides.' },
    { at: 12, text: 'A logic change makes them stale, so reviewers re-read only what matters. Landing records which review shipped each changeset.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Reviews that know what changed', { alpha: g.vis(0) });

    // REVIEWERS file.
    const lx = 110;
    g.alpha(g.vis(0.3), () => {
      g.card(lx, 300, 520, 200, { title: 'pay/REVIEWERS', color: C.amber });
      g.code([
        [['group:payments', C.ink]],
        [['!trivial any', C.violet], ['   # docs-only: anyone', C.dim]],
      ], lx + 30, 392, { size: 22 });
    });

    // Classification ladder.
    const classes = ['identical', 'whitespace', 'comments', 'docs', 'logic'];
    const current = t < 6 ? 4 : t < 11.6 ? 2 : 4;
    g.alpha(g.vis(1.0), () => {
      g.card(lx, 530, 520, 350, { title: 'change class', color: C.violet });
      classes.forEach((c, i) => {
        const yy = 610 + i * 52;
        const on = i === current && t > 1.5;
        const keeps = i <= 2;
        g.box(lx + 30, yy - 30, 460, 42, {
          fill: on ? g.rgba(keeps ? C.mint : C.coral, 0.2) : 'rgba(0,0,0,0)',
          stroke: on ? g.rgba(keeps ? C.mint : C.coral, 0.9) : C.lineSoft, radius: 8,
        });
        g.text(c, lx + 52, yy - 2, { size: 22, font: 'mono', color: on ? C.ink : C.muted });
        g.text(keeps ? 'approval carries over' : 'needs fresh approval', lx + 470, yy - 2, { size: 17, color: keeps ? C.mint : C.coral, align: 'right', alpha: on ? 1 : 0.5 });
      });
    });

    // Revisions of review R7.
    const rx = 700;
    g.alpha(g.vis(0.6), () => {
      g.text('R7  pay: charge customers twice', rx, 330, { size: 28, font: 'display', weight: 700, color: C.ink });
    });
    const revs = [
      {
        at: 1.2, n: 'r1', cls: 'logic', col: C.coral,
        diff: [['- ', C.coral, 'return 1'], ['+ ', C.mint, 'return 2']],
      },
      {
        at: 6.2, n: 'r2', cls: 'comments', col: C.mint,
        diff: [['- ', C.coral, '// Charge charges.'], ['+ ', C.mint, '// Charge charges twice.']],
      },
      {
        at: 11.8, n: 'r3', cls: 'logic', col: C.coral,
        diff: [['- ', C.coral, 'return 2'], ['+ ', C.mint, 'return 3']],
      },
    ];
    const cw = 350, gap = 20;
    revs.forEach((r, i) => {
      const x = rx + i * (cw + gap), y = 370;
      g.alpha(g.at(r.at, 0.45, 'out'), () => {
        g.card(x, y, cw, 230, { title: r.n, color: C.blue });
        g.chip(r.cls, x + cw - 20, y + 30, { color: r.col, size: 17, align: 'right' });
        r.diff.forEach(([sign, col, text], j) => {
          g.text(sign + text, x + 24, y + 100 + j * 40, { size: 19, font: 'mono', color: col, maxWidth: cw - 44 });
        });
      });
    });

    // Approval state under the revisions.
    const ay = 660;
    g.alpha(g.vis(2.6), () => g.card(rx, ay, 1110, 220, { title: 'approvals', color: C.mint }));
    const approvalLine = (x, label, col, start, end) => g.alpha(g.vis(start, end), () => {
      g.chip(label, x, ay + 100, { color: col, size: 20 });
    });
    approvalLine(rx + 30, 'carol ✓ r1', C.mint, 3.2);
    approvalLine(rx + 30 + cw + gap, 'carried over to r2', C.mint, 7.4, 11.8);
    approvalLine(rx + 30 + cw + gap, 'stale after r3', C.coral, 12.6);
    approvalLine(rx + 30 + 2 * (cw + gap), 'carol ✓ r3', C.mint, 14.2);

    const status = t < 3.2 ? ['/pay needs 1 approval from group:payments', C.coral]
      : t < 12.6 ? ['approved · ready to land', C.mint]
        : t < 14.2 ? ['/pay needs 1 approval from group:payments', C.coral]
          : t < 15.6 ? ['approved · ready to land', C.mint]
            : ['landed as 7d78055e2afa · review R7', C.teal];
    g.alpha(g.vis(2.8), () => {
      g.text(status[0], rx + 30, ay + 168, { size: 22, font: 'mono', color: status[1] });
    });
  },
});

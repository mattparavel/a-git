// Scene 11: reviews with owners and semantic approval carry-over.
AGX.scene({
  id: 'review',
  order: 11,
  title: 'Semantic code review',
  duration: 21,
  captions: [
    { at: 0, text: 'A REVIEWERS file names who owns each directory. Carol, in group:payments, approves R7.' },
    { at: 6.2, text: 'Each revision is classified against the last. Comment or whitespace edits keep approvals.' },
    { at: 12.2, text: 'A logic change makes them stale, so carol re-reads it. Landing records which review shipped it.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Reviews that know what changed', { alpha: g.vis(0) });

    // REVIEWERS file. The path is drawn in mono (card titles are uppercased).
    const lx = 110;
    g.alpha(g.vis(0.3), () => {
      g.card(lx, 300, 520, 200, { color: C.amber });
      g.text('pay/REVIEWERS', lx + 22, 336, { size: 20, font: 'mono', weight: 600, color: C.amber });
      g.code([
        [['group:payments', C.ink]],
        [['!trivial any', C.violet]],
        [['# trivial: 1 from any non-author', C.muted]],
      ], lx + 30, 390, { size: 22 });
    });

    // Classification ladder. Rows light up only when a revision's interdiff
    // (its change against the previous revision) has been classified.
    const classes = ['identical', 'whitespace', 'comments', 'docs', 'logic'];
    const lit = [0, 0, g.vis(8.0, 13.2), 0, g.vis(13.6)];
    const rowY = (i) => 596 + i * 52; // top of row box
    g.alpha(g.vis(5.8), () => {
      g.card(lx, 530, 520, 350, { title: 'change class', color: C.violet });
      classes.forEach((c, i) => {
        const top = rowY(i);
        const a = lit[i];
        const keeps = i <= 2;
        const col = keeps ? C.mint : C.coral;
        g.box(lx + 30, top, 460, 42, { stroke: C.lineSoft, radius: 8 });
        if (a > 0.001) {
          g.box(lx + 30, top, 460, 42, { fill: g.rgba(col, 0.2 * a), stroke: g.rgba(col, 0.9 * a), radius: 8 });
        }
        g.text(c, lx + 52, top + 28, { size: 22, font: 'mono', color: g.mix(C.muted, C.ink, a) });
        g.text(keeps ? 'approval carries over' : 'needs fresh approval', lx + 470, top + 28,
          { size: 18, color: col, align: 'right', alpha: 0.75 + 0.25 * a });
      });
    });

    // Revisions of review R7.
    const rx = 700;
    g.alpha(g.vis(0.6), () => {
      g.text('R7  pay: charge customers twice', rx, 330, { size: 28, font: 'display', weight: 700, color: C.ink });
    });
    const revs = [
      {
        at: 1.2, n: 'rev 1', cls: 'first', col: C.blue,
        diff: [['- ', C.coral, 'return 1'], ['+ ', C.mint, 'return 2']],
      },
      {
        at: 6.6, n: 'rev 2', cls: 'vs rev 1: comments', col: C.mint,
        diff: [['- ', C.coral, '// Charge charges.'], ['+ ', C.mint, '// Charge charges twice.']],
      },
      {
        at: 12.4, n: 'rev 3', cls: 'vs rev 2: logic', col: C.coral,
        diff: [['- ', C.coral, 'return 2'], ['+ ', C.mint, 'return 3']],
      },
    ];
    const cw = 350, gap = 20, ry = 370, rh = 190;
    revs.forEach((r, i) => {
      const x = rx + i * (cw + gap);
      g.alpha(g.at(r.at, 0.45, 'out'), () => {
        g.card(x, ry, cw, rh, { title: r.n, color: C.blue });
        g.chip(r.cls, x + cw - 20, ry + 30, { color: r.col, size: 17, align: 'right' });
        r.diff.forEach(([sign, col, text], j) => {
          g.text(sign + text, x + 24, ry + 100 + j * 40, { size: 19, font: 'mono', color: col, maxWidth: cw - 44 });
        });
      });
    });

    // Interdiff arrows: from a revision card, through the gap above the
    // approvals card, into the ladder row the classifier picked.
    const interdiff = (i, row, start, end) => {
      const cx = rx + i * (cw + gap) + cw / 2;
      const yMid = 610, xGut = 665, yRow = rowY(row) + 21;
      const pts = [[cx, ry + rh + 2], [cx, yMid], [xGut, yMid], [xGut, yRow], [lx + 30 + 460 + 6, yRow]];
      g.alpha(g.vis(start, end), () => {
        g.path(pts, { color: C.violet, width: 2.5, head: true, progress: g.at(start, 0.7, 'inOut') });
        g.alpha(g.vis(start + 0.1), () => g.chip('interdiff', cx + 14, 586, { color: C.violet, size: 17 }));
      });
    };
    interdiff(1, 2, 7.3, 11.8);
    interdiff(2, 4, 12.9);

    // Approval state under the revisions.
    const ay = 660, cy = ay + 100;
    const col = (i) => rx + 30 + i * (cw + gap);
    g.alpha(g.vis(2.2), () => g.card(rx, ay, 1090, 220, { title: 'approvals', color: C.mint }));
    // An approval chip that can later turn stale (dimmed coral, struck through).
    const approval = (x, label, color, start, end, staleAt) => g.alpha(g.vis(start, end), () => {
      const s = staleAt === undefined ? 0 : g.vis(staleAt);
      g.alpha(1 - s, () => g.chip(label, x, cy, { color, size: 20 }));
      g.alpha(s, () => {
        const w = g.chip(label, x, cy, { color: C.coral, size: 20, alpha: 0.7 });
        g.line(x + 10, cy, x + w - 10, cy, { color: C.coral, width: 1.5, alpha: 0.85 });
      });
    });
    approval(col(0), 'carol ✓ rev 1', C.mint, 4.3, null, 14.1);
    approval(col(1), 'carried over to rev 2', C.mint, 8.8, null, 14.1);
    approval(col(2), 'stale after rev 3', C.coral, 14.1, 15.9);
    approval(col(2), 'carol ✓ rev 3', C.mint, 16.3);

    // Status line, as `agit review show` / `agit review land` print it.
    const needs = '=> /pay needs 1 approval(s) from group:payments, has 0';
    const ok = '=> approved, ready to land';
    const status = [
      [needs, C.coral, 2.6, 4.75],
      [ok, C.mint, 5.0, 14.15],
      [needs, C.coral, 14.4, 16.45],
      [ok, C.mint, 16.7, 17.95],
      ['landed review 7 on main: 3f9c2a1b8e04 -> 7d78055e2afa', C.teal, 18.2, null],
    ];
    status.forEach(([str, color, start, end]) => g.alpha(g.vis(start, end, 0.25), () => {
      g.text(str, rx + 30, ay + 168, { size: 22, font: 'mono', color });
    }));
  },
});

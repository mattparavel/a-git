// Scene 12: git import via fast-export and export via fast-import.
AGX.scene({
  id: 'git',
  order: 12,
  title: 'Working with git',
  duration: 12,
  captions: [
    { at: 0, text: 'Existing history comes in through git fast-export, which already describes each commit as changes against its first parent.' },
    { at: 6, text: 'It goes back out through fast-import, and the round trip reproduces the exact same git commit hashes.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Bring your git history along', { alpha: g.vis(0) });

    const y = 440;
    const G1 = { x: 260, y }, A = { x: 960, y }, G2 = { x: 1660, y };
    g.alpha(g.vis(0.2), () => g.node(G1.x, y, { icon: 'git', label: 'git repo', color: C.muted, w: 240, h: 190, labelFont: 'body' }));
    g.alpha(g.vis(0.4), () => g.node(A.x, y, { icon: 'server', label: 'a-git', sub: 'changesets', color: C.teal, w: 240, h: 190 }));
    g.alpha(g.vis(5.8), () => g.node(G2.x, y, { icon: 'git', label: 'git mirror', color: C.muted, w: 240, h: 190, labelFont: 'body' }));

    g.arrow(G1.x + 125, y, A.x - 125, y, { color: C.amber, width: 3, progress: g.at(1.0, 1.0) });
    g.alpha(g.vis(1.4), () => g.text('agit import-git', (G1.x + A.x) / 2, y - 24, { size: 21, font: 'mono', color: C.amber, align: 'center' }));
    g.arrow(A.x + 125, y, G2.x - 125, y, { color: C.amber, width: 3, progress: g.at(6.4, 1.0) });
    g.alpha(g.vis(6.8), () => g.text('agit export-git', (A.x + G2.x) / 2, y - 24, { size: 21, font: 'mono', color: C.amber, align: 'center' }));

    // A fast-export excerpt next to the changeset it becomes.
    g.alpha(g.vis(1.8), () => {
      g.card(110, 600, 560, 280, { title: 'git fast-export', color: C.muted });
      g.code([
        [['commit refs/heads/main', C.ink]],
        [['from :5', C.blue]],
        [['M 100644 :7 src/a.go', C.amber]],
        [['D old.txt', C.coral]],
      ], 140, 690, { size: 22, progress: g.at(2.0, 1.6, 'linear') });
    });
    g.alpha(g.vis(3.6), () => {
      g.arrow(690, 740, 760, 740, { color: C.line, width: 3 });
      g.card(780, 600, 460, 280, { title: 'changeset', color: C.amber });
      g.code([
        [['parents ', C.muted], ['[first parent]', C.blue]],
        [['changes', C.muted]],
        [['  src/a.go → blob', C.amber]],
        [['  old.txt  → delete', C.coral]],
      ], 810, 690, { size: 22 });
    });

    // Hash equality badge.
    g.alpha(g.vis(8.2), () => {
      g.card(1300, 600, 510, 280, { title: 'round trip', color: C.mint });
      g.text('original', 1330, 690, { size: 19, color: C.muted });
      g.text('470870d5fe13', 1330, 724, { size: 26, font: 'mono', color: C.ink });
      g.text('mirror', 1330, 782, { size: 19, color: C.muted });
      g.text('470870d5fe13', 1330, 816, { size: 26, font: 'mono', color: C.ink });
      g.icon('check', 1720, 760, 52, { color: C.mint, stroke: 5 });
    });
  },
});

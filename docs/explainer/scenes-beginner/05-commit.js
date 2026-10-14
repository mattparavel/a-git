// Beginner 5: committing goes straight to the server; a changeset is a list of edits.
AGX.scene({
  id: 'commit',
  order: 5,
  title: 'Committing',
  duration: 26,
  captions: [
    { at: 0, text: "Committing works a little differently. When you commit in a-git, your change goes straight to the server. There's no separate push." },
    { at: 9.6, text: 'Each commit, called a changeset, records only what you changed: which files were edited, added or removed.' },
    { at: 17.6, text: "The server fits those edits onto the previous version to get the full picture, reusing everything that didn't change." },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Commit goes straight to the server', { alpha: g.vis(0) });

    // You commit; the change travels to the server in one step.
    g.alpha(g.vis(0.3), () => BK.person(g, 250, 420, { name: 'You', color: C.blue }));
    g.alpha(g.vis(1.0), () => g.chip('agit commit', 250, 580, { color: C.blue, size: 26, align: 'center' }));
    g.alpha(g.vis(0.6), () => {
      g.node(1580, 420, { icon: 'server', label: 'a-git server', color: C.teal, w: 260, h: 190, labelFont: 'body', labelSize: 24 });
    });
    const go = g.at(2.0, 1.6, 'inOut');
    g.arrow(340, 420, 1440, 420, { color: C.teal, width: 4, progress: go, headSize: 18 });
    g.packet([[340, 420], [1440, 420]], g.at(2.0, 1.6, 'inOut'), { color: C.amber, r: 12 });
    // The old two-step flow, crossed out.
    g.alpha(g.vis(4.2), () => {
      const x = 760, y = 340;
      g.text('commit, then push later', x, y, { size: 26, color: C.dim });
      const w = g.measure('commit, then push later', { size: 26 });
      g.line(x - 6, y - 9, x + w + 6, y - 9, { color: C.coral, width: 3, progress: g.at(4.6, 0.5) });
      g.text('one step', x + w + 30, y, { size: 26, weight: 600, color: C.mint, alpha: g.at(5.2, 0.4) });
    });

    // The changeset: a short list of edits, not a copy of every file.
    const cardIn = g.vis(9.8);
    g.alpha(cardIn, () => {
      g.card(160, 520, 560, 360, { title: 'changeset', color: C.amber });
      const rows = [['edited', 'app.go', 'edited', C.amber], ['added', 'helper.go', 'added', C.mint], ['removed', 'old.md', 'removed', C.coral]];
      rows.forEach(([verb, name, st, col], i) => {
        g.alpha(g.at(10.6 + i * 0.7, 0.4), () => {
          BK.file(g, 330, 630 + i * 78, name, { state: st, w: 260 });
          g.text(verb, 490, 638 + i * 78, { size: 24, weight: 600, color: col });
        });
      });
      g.text('only what changed', 190, 860, { size: 21, color: C.muted });
    });

    // The server fits the edits onto the previous version.
    const prev = ['README.md', 'app.go', 'old.md', 'logo.png', 'notes.txt'];
    const next = [['README.md', 'same'], ['app.go', 'edited'], ['helper.go', 'added'], ['logo.png', 'same'], ['notes.txt', 'same']];
    g.alpha(g.vis(17.8), () => {
      g.text('previous version', 900, 560, { size: 22, color: C.muted });
      prev.forEach((f, i) => BK.file(g, 1000, 610 + i * 62, f, { state: 'same', w: 200, h: 50, size: 19 }));
      g.text('new version', 1400, 560, { size: 22, color: C.muted });
    });
    next.forEach(([f, st], i) => {
      const a = g.at(18.8 + i * 0.5, 0.45);
      g.alpha(a, () => {
        const reused = st === 'same';
        if (reused) {
          const j = prev.indexOf(f);
          g.line(1100, 610 + j * 62, 1400, 610 + i * 62, { color: g.rgba(C.teal, 0.5), width: 2, dash: [6, 7] });
        }
        BK.file(g, 1500, 610 + i * 62, f, { state: st, w: 200, h: 50, size: 19 });
      });
    });
    g.alpha(g.vis(21.6), () => {
      g.text('dashed lines: reused, not copied', 1300, 925 - 40, { size: 21, color: C.teal, align: 'center' });
    });
  },
});

// Scene 12: git import via fast-export and export via fast-import.
AGX.scene({
  id: 'git',
  order: 12,
  title: 'Working with git',
  duration: 13,
  captions: [
    { at: 0, text: 'History comes in through git fast-export, already written as changes against the first parent.' },
    { at: 6.3, text: 'It goes back out through git fast-import; unsigned history keeps its exact commit hashes.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Bring your git history along', { alpha: g.vis(0) });

    // Three columns: each node sits centred over the card below it.
    // Cards are 510 wide with equal 85 px gaps across the safe area.
    const y = 440;
    const G1 = { x: 365, y }, A = { x: 960, y }, G2 = { x: 1555, y };
    const NW = 240, NH = 190;
    g.alpha(g.vis(0.2), () => g.node(G1.x, y, { icon: 'git', label: 'git repo', color: C.muted, w: NW, h: NH, labelFont: 'body' }));
    g.alpha(g.vis(0.4), () => g.node(A.x, y, { icon: 'server', label: 'a-git', sub: 'changesets', color: C.teal, w: NW, h: NH }));
    g.alpha(g.vis(6.1), () => {
      g.node(G2.x, y, { icon: 'git', label: 'git mirror', color: C.muted, w: NW, h: NH, labelFont: 'body' });
      // Same hashes: the mirror lights up as a success.
      g.alpha(g.vis(9.3), () => g.node(G2.x, y, { icon: 'git', label: 'git mirror', color: C.mint, active: true, w: NW, h: NH, labelFont: 'body' }));
    });

    // Import arrow (drawn only once it starts, so no stray end-cap dot).
    const imp = g.at(1.0, 1.0);
    if (imp > 0) g.arrow(G1.x + NW / 2 + 5, y, A.x - NW / 2 - 5, y, { color: C.amber, width: 3, progress: imp });
    const impX = (G1.x + A.x) / 2;
    g.alpha(g.vis(1.4), () => g.text('agit import-git', impX, y - 24, { size: 21, font: 'mono', color: C.amber, align: 'center' }));
    g.alpha(g.vis(1.6), () => g.text('via git fast-export', impX, y + 30, { size: 18, font: 'mono', color: C.muted, align: 'center' }));

    // Export arrow.
    const exp = g.at(6.4, 1.0);
    if (exp > 0) g.arrow(A.x + NW / 2 + 5, y, G2.x - NW / 2 - 5, y, { color: C.amber, width: 3, progress: exp });
    const expX = (A.x + G2.x) / 2;
    g.alpha(g.vis(6.8), () => g.text('agit export-git', expX, y - 24, { size: 21, font: 'mono', color: C.amber, align: 'center' }));
    g.alpha(g.vis(7.0), () => g.text('via git fast-import', expX, y + 30, { size: 18, font: 'mono', color: C.muted, align: 'center' }));

    // Card geometry.
    const CW = 510, CH = 280, CY = 600;
    const X1 = G1.x - CW / 2, X2 = A.x - CW / 2, X3 = G2.x - CW / 2;
    const FS = 22, LH = Math.round(FS * 1.55), ROW0 = 690;
    const mono = { size: FS, font: 'mono' };
    const mw = (s) => g.measure(s, mono);
    // Soft highlight behind one row of code, pulsed in pairs across the two cards.
    const hl = (x, row, str, color, a) => {
      if (a <= 0.001) return;
      const by = ROW0 + row * LH;
      g.box(x - 6, by - 21, mw(str) + 12, 28, { fill: g.rgba(color, 0.18 * a), stroke: g.rgba(color, 0.6 * a), lineWidth: 1.5, radius: 7 });
    };
    // Pair timings: first parent, then the modify, then the delete.
    const hParent = g.vis(4.0, 4.9), hMod = g.vis(5.0, 5.6), hDel = g.vis(5.7, 6.3);

    // A fast-export excerpt next to the changeset it becomes.
    // fast-export and a-git both list file changes sorted by path.
    g.alpha(g.vis(1.8), () => {
      g.card(X1, CY, CW, CH, { title: 'git fast-export', color: C.muted });
      const cx = X1 + 30;
      hl(cx, 1, 'from :5', C.blue, hParent);
      hl(cx, 2, 'M 100644 :7 src/a.go', C.amber, hMod);
      hl(cx, 3, 'D src/old.go', C.coral, hDel);
      g.code([
        [['commit refs/heads/main', C.ink]],
        [['from :5', C.blue]],
        [['M 100644 :7 src/a.go', C.amber]],
        [['D src/old.go', C.coral]],
      ], cx, ROW0, { size: FS, progress: g.at(2.0, 1.6, 'linear') });
    });

    const conn = g.at(3.3, 0.4);
    if (conn > 0) g.arrow(X1 + CW + 12, CY + 140, X2 - 12, CY + 140, { color: C.amber, width: 3, headSize: 16, progress: conn });

    g.alpha(g.vis(3.6), () => {
      g.card(X2, CY, CW, CH, { title: 'changeset', color: C.amber });
      const cx = X2 + 30;
      hl(cx + mw('parents '), 0, '[id of :5]', C.blue, hParent);
      hl(cx + mw('  '), 2, 'src/a.go   → blob', C.amber, hMod);
      hl(cx + mw('  '), 3, 'src/old.go → delete', C.coral, hDel);
      g.code([
        [['parents ', C.muted], ['[id of :5]', C.blue]],
        [['changes', C.muted]],
        [['  src/a.go   → blob', C.amber]],
        [['  src/old.go → delete', C.coral]],
      ], cx, ROW0, { size: FS });
    });

    // Hash equality: the mirror's hash types out, then matches.
    const HASH = '470870d5fe13';
    g.alpha(g.vis(7.4), () => {
      g.card(X3, CY, CW, CH, { title: 'round trip', color: C.mint });
      const tx = X3 + 30;
      const match = g.vis(9.3);
      g.alpha(g.vis(7.6), () => {
        g.text('original', tx, 684, { size: 19, color: C.muted });
        g.text(HASH, tx, 720, { size: 26, font: 'mono', color: C.ink });
        if (match > 0) g.text(HASH, tx, 720, { size: 26, font: 'mono', color: C.mint, alpha: match });
      });
      g.alpha(g.vis(8.0), () => {
        g.text('mirror', tx, 768, { size: 19, color: C.muted });
        g.code([[[HASH, C.ink]]], tx, 804, { size: 26, progress: g.at(8.1, 0.9, 'linear') });
        if (match > 0) g.text(HASH, tx, 804, { size: 26, font: 'mono', color: C.mint, alpha: match });
      });
      const s = g.at(9.3, 0.35, 'outBack');
      if (s > 0) g.transform({ scale: s, cx: X3 + CW - 90, cy: 750 }, () => g.icon('check', X3 + CW - 90, 750, 52, { color: C.mint, stroke: 5 }));
      g.alpha(g.vis(9.9), () => g.text('unsigned commits, no submodules', tx, 852, { size: 18, color: C.muted }));
    });
  },
});

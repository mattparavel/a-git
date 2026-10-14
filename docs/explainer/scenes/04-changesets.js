// Scene 4: changesets list only changes; the root tree is derived by
// rewriting only the directories on changed paths.
AGX.scene({
  id: 'changesets',
  order: 4,
  title: 'Changesets',
  duration: 18,
  captions: [
    { at: 0, text: 'The unit of work is a changeset. Instead of pointing at a full snapshot, it lists only the files that changed since its first parent.' },
    { at: 7, text: "Files and directories are content-addressed: each object's ID is the SHA-256 of its contents." },
    { at: 12, text: 'The server derives the new root tree by rewriting only the directories on changed paths. Everything else is shared with the parent.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('A changeset lists only what changed', { alpha: g.vis(0) });

    // ---- the changeset card (left) ----
    const x = 110, y = 290, w = 690, h = 560;
    g.alpha(g.vis(0.2), () => g.card(x, y, w, h, { title: 'changeset', color: C.amber }));
    g.alpha(g.vis(0.4), () => g.chip('99e0ba9acf52', x + w - 24, y + 30, { color: C.amber, size: 20, align: 'right' }));

    const rows = [
      ['parents', [['5ea84d4734f0', C.amber]]],
      ['author', [['Ada <ada@example.com>', C.ink]]],
      ['message', [['app: return 2 and add a helper', C.ink]]],
    ];
    rows.forEach(([k, segs], i) => {
      g.alpha(g.stagger(i, 0.7, 0.25, 0.4), () => {
        g.text(k, x + 28, y + 100 + i * 50, { size: 21, color: C.muted, font: 'mono' });
        g.code([segs], x + 170, y + 100 + i * 50, { size: 21 });
      });
    });
    g.alpha(g.vis(1.6), () => {
      g.text('changes', x + 28, y + 262, { size: 21, color: C.muted, font: 'mono' });
      g.text('relative to the first parent', x + 170, y + 262, { size: 19, color: C.dim });
    });
    const changes = [
      ['M', 'src/app/app.go', '7c1e09d2', C.amber],
      ['A', 'src/app/helper.go', '2d44b1fe', C.mint],
      ['D', 'docs/old.md', null, C.coral],
    ];
    changes.forEach(([op, p, blob, col], i) => {
      const yy = y + 322 + i * 58;
      g.alpha(g.stagger(i, 2.2, 0.45, 0.4), () => {
        g.box(x + 28, yy - 30, w - 56, 46, { fill: g.rgba(col, 0.08), stroke: g.rgba(col, 0.35), radius: 8 });
        g.text(op, x + 48, yy + 1, { size: 24, font: 'mono', weight: 600, color: col });
        g.text(p, x + 88, yy + 1, { size: 22, font: 'mono', color: C.ink });
        if (blob) g.text('blob ' + blob + '…', x + w - 48, yy + 1, { size: 19, font: 'mono', color: C.amber, align: 'right' });
        else g.text('deleted', x + w - 48, yy + 1, { size: 19, font: 'mono', color: C.coral, align: 'right' });
      });
    });
    g.alpha(g.vis(4.2), () => {
      g.text('no tree pointer: the root tree is derived', x + 28, y + h - 30, { size: 20, color: C.violet });
    });

    // ---- the tree (right) ----
    // Nodes: [id, label, x, y, parent, kind]
    const N = {
      root: { label: '/', x: 1330, y: 340 },
      docs: { label: 'docs/', x: 1000, y: 470, parent: 'root' },
      src: { label: 'src/', x: 1330, y: 470, parent: 'root' },
      tools: { label: 'tools/', x: 1680, y: 470, parent: 'root' },
      old: { label: 'old.md', x: 920, y: 600, parent: 'docs', file: true },
      guide: { label: 'guide.md', x: 1080, y: 600, parent: 'docs', file: true },
      app: { label: 'app/', x: 1220, y: 600, parent: 'src' },
      lib: { label: 'lib/', x: 1450, y: 600, parent: 'src' },
      build: { label: 'build.sh', x: 1680, y: 600, parent: 'tools', file: true },
      appgo: { label: 'app.go', x: 1140, y: 740, parent: 'app', file: true },
      helper: { label: 'helper.go', x: 1300, y: 740, parent: 'app', file: true },
      libgo: { label: 'lib.go', x: 1450, y: 740, parent: 'lib', file: true },
    };
    const treeIn = g.vis(4.6);
    const apply = g.at(8.2, 0.8);            // file-level changes land
    const wave = (k) => g.at(9.4 + k * 0.9, 0.6, 'out'); // rewrite climbs the path
    const rewriteStep = { app: 0, docs: 0, src: 1, root: 2 };
    const statusOf = (id) => {
      if (id === 'old') return apply > 0 ? 'deleted' : 'plain';
      if (id === 'helper') return apply > 0 ? 'added' : 'hidden';
      if (id === 'appgo') return apply > 0 ? 'modified' : 'plain';
      if (id in rewriteStep) return wave(rewriteStep[id]) > 0 ? 'rewritten' : 'plain';
      return t > 12.6 ? 'shared' : 'plain';
    };

    g.alpha(treeIn, () => {
      // Edges first.
      for (const [id, n] of Object.entries(N)) {
        if (!n.parent) continue;
        const p = N[n.parent];
        const st = statusOf(id);
        let a = 1;
        if (st === 'hidden') a = 0;
        if (id === 'helper') a = apply;
        if (id === 'old') a = 1 - g.at(9.8, 0.8);
        g.line(p.x, p.y + 22, n.x, n.y - 22, { color: C.line, width: 2, alpha: a });
      }
      for (const [id, n] of Object.entries(N)) {
        const st = statusOf(id);
        let col = C.teal, a = 1, fill = C.panel;
        if (st === 'hidden') a = 0;
        if (st === 'added') { col = C.mint; a = apply; fill = g.rgba(C.mint, 0.18); }
        if (st === 'modified') { col = C.amber; fill = g.rgba(C.amber, 0.2 * apply); }
        if (st === 'deleted') { col = C.coral; a = 1 - g.at(9.8, 0.8); fill = g.rgba(C.coral, 0.2); }
        if (st === 'rewritten') { col = C.amber; fill = g.rgba(C.amber, 0.2); }
        if (st === 'shared') { col = C.teal; a = 0.55; }
        const bw = Math.max(96, g.measure(n.label, { size: 20, font: 'mono' }) + 34);
        g.alpha(a, () => {
          g.box(n.x - bw / 2, n.y - 22, bw, 44, { fill, stroke: g.rgba(col, 0.85), radius: n.file ? 22 : 8, lineWidth: 2 });
          g.text(n.label, n.x, n.y + 7, { size: 20, font: 'mono', color: C.ink, align: 'center' });
          if (st === 'rewritten' || st === 'modified' || st === 'added') {
            g.text(g.hash('new-' + id, 6), n.x, n.y + 46, { size: 16, font: 'mono', color: col, align: 'center' });
          }
        });
      }
      // Content addressing callout.
      g.alpha(g.vis(7.2, 11.6), () => {
        g.box(870, 790, 560, 70, { fill: C.panel2, stroke: g.rgba(C.amber, 0.5), radius: 10 });
        g.text('id = sha256( "blob 12\\0" + content )', 894, 834, { size: 21, font: 'mono', color: C.amber });
      });
      // Shared vs rewritten tally.
      g.alpha(g.vis(13.2), () => {
        g.chip('4 trees rewritten', 900, 830, { color: C.amber, size: 21 });
        g.chip('lib/, tools/ and guide.md shared', 1140, 830, { color: C.teal, size: 21 });
      });
    });
  },
});

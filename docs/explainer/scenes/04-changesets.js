// Scene 4: changesets list only changes; the root tree is derived by
// rewriting only the directories on changed paths.
AGX.scene({
  id: 'changesets',
  order: 4,
  title: 'Changesets',
  duration: 21,
  captions: [
    { at: 0, text: 'A changeset is the unit of work. It lists only the files changed since its first parent, not a full snapshot.' },
    { at: 7.0, text: 'Files and directories are content-addressed by the SHA-256 of their kind, size and contents.' },
    { at: 13.0, text: 'The server derives the new root tree, rewriting only directories on changed paths. The rest is shared with the parent.' },
  ],
  draw(g, t) {
    const C = g.C;
    // IDs are shown the way a-git prints them (ID.Short(): 12 hex characters).
    const SELF = '99e0ba9acf52', PARENT = '5ea84d4734f0';
    const BLOB = { appgo: '7c1e09d2b4a1', helper: '2d44b1fe0c93' };
    g.heading('A changeset lists only what changed', { alpha: g.vis(0) });

    // ---- the changeset card (left) ----
    const x = 110, y = 290, w = 690, h = 560;
    g.alpha(g.vis(0.2), () => g.card(x, y, w, h, { title: 'changeset', color: C.amber }));
    g.alpha(g.vis(0.4), () => g.chip(SELF, x + w - 24, y + 30, { color: C.amber, size: 20, align: 'right' }));

    const rows = [
      ['parents', [[PARENT, C.amber]]],
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
      g.text('relative to the first parent', x + 170, y + 262, { size: 19, color: C.muted });
    });
    // A change is {path, mode, blob} in canonical path order; no mode means delete.
    const changes = [
      ['', 'docs/old.md', null, C.coral],
      ['100644', 'src/app/app.go', BLOB.appgo, C.amber],
      ['100644', 'src/app/helper.go', BLOB.helper, C.mint],
    ];
    changes.forEach(([mode, p, blob, col], i) => {
      const yy = y + 322 + i * 58;
      g.alpha(g.stagger(i, 2.2, 0.45, 0.4), () => {
        g.box(x + 28, yy - 30, w - 56, 46, { fill: g.rgba(col, 0.08), stroke: g.rgba(col, 0.35), radius: 8 });
        if (mode) g.text(mode, x + 46, yy + 1, { size: 20, font: 'mono', color: col });
        g.text(p, x + 142, yy + 1, { size: 22, font: 'mono', color: C.ink });
        if (blob) g.text('blob ' + blob, x + w - 46, yy + 1, { size: 19, font: 'mono', color: C.amber, align: 'right' });
        else g.text('no mode = delete', x + w - 46, yy + 1, { size: 19, font: 'mono', color: C.coral, align: 'right' });
      });
    });
    g.alpha(g.vis(4.2), () => {
      g.text('no tree pointer: the root tree is derived', x + 28, y + h - 30, { size: 20, color: C.violet });
    });

    // ---- the tree (right) ----
    const N = {
      root: { label: '/', x: 1375, y: 340 },
      docs: { label: 'docs/', x: 998, y: 470, parent: 'root' },
      src: { label: 'src/', x: 1375, y: 470, parent: 'root' },
      tools: { label: 'tools/', x: 1710, y: 470, parent: 'root' },
      old: { label: 'old.md', x: 920, y: 600, parent: 'docs', file: true },
      guide: { label: 'guide.md', x: 1075, y: 600, parent: 'docs', file: true },
      app: { label: 'app/', x: 1235, y: 600, parent: 'src' },
      lib: { label: 'lib/', x: 1515, y: 600, parent: 'src' },
      build: { label: 'build.sh', x: 1710, y: 600, parent: 'tools', file: true },
      appgo: { label: 'app.go', x: 1155, y: 740, parent: 'app', file: true },
      helper: { label: 'helper.go', x: 1320, y: 740, parent: 'app', file: true },
      libgo: { label: 'lib.go', x: 1515, y: 740, parent: 'lib', file: true },
    };
    // New IDs: a changed file's tree entry is exactly the blob from the change;
    // only the rewritten trees get fresh IDs.
    const ID = {
      appgo: BLOB.appgo,
      helper: BLOB.helper,
      app: g.hash('new-app', 12),
      docs: g.hash('new-docs', 12),
      src: g.hash('new-src', 12),
      root: g.hash('new-root', 12),
    };
    const treeIn = g.vis(2.4);
    const apply = g.at(8.3, 0.8);              // file-level changes land
    const gone = g.at(9.9, 0.8);               // the deleted file leaves docs/
    const wave = (k) => g.at(13.3 + k * 0.8, 0.6, 'out'); // rewrite climbs the path
    const rewriteStep = { app: 0, docs: 0, src: 1, root: 2 };
    const share = g.at(15.8, 0.5);             // untouched entries are reused
    const pulse = g.at(17.0, 1.4, 'pulse');    // highlight the reused entries
    const SHARED = ['tools', 'lib', 'guide'];
    const look = (id) => {
      if (id === 'old') return { col: C.coral, u: apply, a: 1 - gone };
      if (id === 'helper') return { col: C.mint, u: apply, a: apply };
      if (id === 'appgo') return { col: C.amber, u: apply, a: 1 };
      if (id in rewriteStep) return { col: C.violet, u: wave(rewriteStep[id]), a: 1 };
      return { col: C.muted, u: 0, a: 1 - 0.45 * share };
    };

    g.alpha(treeIn, () => {
      // Which tree this is: the parent's snapshot, then the derived one.
      // Fade the old label out before the new one comes in, so they never overlap.
      g.text('root tree of parent ' + PARENT, 870, 326, { size: 18, font: 'mono', color: C.muted, alpha: 1 - g.at(13.1, 0.3) });
      g.text('root tree of ' + SELF + ' (derived)', 870, 326, { size: 18, font: 'mono', color: C.violet, alpha: g.at(13.4, 0.4) });

      // Edges first.
      for (const [id, n] of Object.entries(N)) {
        if (!n.parent) continue;
        const p = N[n.parent];
        const ea = id === 'helper' ? apply : id === 'old' ? 1 - gone : 1;
        g.line(p.x, p.y + 22, n.x, n.y - 22, { color: C.line, width: 2, alpha: ea });
        if (SHARED.includes(id) && pulse > 0) {
          g.line(p.x, p.y + 22, n.x, n.y - 22, { color: C.ink, width: 3, alpha: 0.7 * pulse });
        }
      }
      for (const [id, n] of Object.entries(N)) {
        const { col, u, a } = look(id);
        const bw = Math.max(96, g.measure(n.label, { size: 20, font: 'mono' }) + 34);
        const r = n.file ? 22 : 8;
        g.alpha(a, () => {
          g.box(n.x - bw / 2, n.y - 22, bw, 44, { fill: C.panel, radius: r });
          g.box(n.x - bw / 2, n.y - 22, bw, 44, {
            fill: g.rgba(col, 0.18 * u),
            stroke: g.rgba(g.mix(C.muted, col, u), 0.6 + 0.25 * u),
            radius: r,
            lineWidth: 2,
          });
          g.text(n.label, n.x, n.y + 7, { size: 20, font: 'mono', color: C.ink, align: 'center' });
          if (ID[id] && u > 0) {
            // Files: below (leaves have no outgoing edges). Trees: to the right.
            if (n.file) g.text(ID[id], n.x, n.y + 42, { size: 18, font: 'mono', color: col, align: 'center', alpha: u });
            else g.text(ID[id], n.x + bw / 2 + 10, n.y + 7, { size: 18, font: 'mono', color: col, alpha: u });
          }
        });
      }
      // Content addressing callout.
      g.alpha(g.vis(7.3, 12.6), () => {
        g.box(870, 808, 640, 64, { fill: C.panel2, stroke: g.rgba(C.amber, 0.5), radius: 10 });
        g.text('id = sha256(kind + " " + size + "\\0" + content)', 894, 850, { size: 21, font: 'mono', color: C.amber });
      });
      // Shared vs rewritten tally.
      g.alpha(g.vis(16.3), () => {
        const w1 = g.chip('4 trees rewritten', 870, 840, { color: C.violet, size: 21 });
        g.chip('lib/, tools/ and guide.md shared', 870 + w1 + 16, 840, { color: C.muted, size: 21 });
      });
    });
  },
});

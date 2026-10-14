// Scene 3: a capable server and thin clients that call APIs.
AGX.scene({
  id: 'server',
  order: 3,
  title: 'A smart server, thin clients',
  duration: 16,
  captions: [
    { at: 0, text: "In a-git the server does the heavy lifting. Commits, merges, rebases and lands all run there, so there's nothing to push." },
    { at: 6, text: 'History, diffs, blame and file reads are API calls. Plenty of jobs never need a checkout at all.' },
    { at: 11, text: 'When you do want files on disk, a working copy is just those files plus a tiny .agit directory.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('The server does the work', { alpha: g.vis(0) });

    // Server panel.
    const px = 1110, py = 300, pw = 700, ph = 580;
    g.alpha(g.vis(0.2), () => {
      g.card(px, py, pw, ph, { title: 'agit-server', color: C.teal, glow: true });
      g.icon('server', px + pw - 70, py + 50, 52, { color: C.teal });
    });
    const writes = ['commit', 'merge', 'rebase', 'land', 'revert'];
    const reads = ['log', 'diff', 'blame', 'file', 'search'];
    g.alpha(g.vis(0.6), () => {
      g.text('writes', px + 40, py + 104, { size: 20, color: C.muted, spacing: 1 });
      g.text('reads', px + 370, py + 104, { size: 20, color: C.muted, spacing: 1 });
    });
    const rowY = (i) => py + 150 + i * 62;
    writes.forEach((w, i) => {
      const lit = (w === 'commit' && t > 3.2 && t < 5) || (w === 'land' && t > 13.6);
      g.alpha(g.stagger(i, 0.8, 0.12, 0.4), () => {
        g.chip(w, px + 40, rowY(i), { color: C.teal, size: 24, fill: lit ? g.rgba(C.teal, 0.45) : null });
      });
    });
    reads.forEach((r, i) => {
      const lit = (r === 'file' && t > 8.6 && t < 10.5) || (r === 'log' && t > 7 && t < 8.5);
      g.alpha(g.stagger(i, 1.2, 0.12, 0.4), () => {
        g.chip(r, px + 370, rowY(i), { color: C.violet, size: 24, fill: lit ? g.rgba(C.violet, 0.45) : null });
      });
    });
    g.alpha(g.vis(1.8), () => {
      g.text('storage · derived data · all writes', px + 40, py + ph - 34, { size: 20, color: C.muted });
    });

    // Three clients on the left.
    const clients = [
      { y: 380, icon: 'agent', label: 'agent', color: C.blue },
      { y: 590, icon: 'ci', label: 'CI job', color: C.blue },
      { y: 800, icon: 'laptop', label: 'developer', color: C.blue },
    ];
    const cx = 250;
    clients.forEach((c, i) => {
      g.alpha(g.stagger(i, 0.3, 0.2, 0.5), () => {
        g.node(cx, c.y, { icon: c.icon, label: c.label, color: c.color, w: 190, h: 150, labelFont: 'body', labelSize: 22 });
      });
    });

    // Calls travelling to the server.
    const call = (y, start, label, color, back) => {
      const pts = [[cx + 100, y], [px - 20, y]];
      const u = g.at(start, 1.2, 'inOut');
      g.line(cx + 100, y, px - 20, y, { color: g.rgba(color, 0.3), width: 2, dash: [5, 9], progress: g.at(start - 0.4, 0.6) });
      g.packet(pts, u, { color, label, size: 20, dy: -30 });
      if (back) {
        const v = g.at(start + 1.5, 1.0, 'inOut');
        g.packet([[px - 20, y + 16], [cx + 100, y + 16]], v, { color: C.amber, r: 7 });
      }
    };
    call(380, 2.0, 'POST /changesets', C.teal, true);
    call(590, 7.4, 'GET /file/main?path=…', C.violet, true);
    call(800, 12.4, 'POST /land', C.teal, true);

    // Annotations under each client lane.
    g.alpha(g.vis(4.4), () => {
      g.chip('commit · no push', cx + 120, 450, { color: C.teal, size: 20 });
    });
    g.alpha(g.vis(9.6), () => {
      g.chip('no checkout needed', cx + 120, 660, { color: C.violet, size: 20 });
    });

    // A working copy: files plus .agit/.
    g.alpha(g.vis(11.2), () => {
      const bx = 420, by = 715, bw = 360;
      g.box(bx, by - 72, bw, 66, { fill: C.panel, stroke: g.rgba(C.blue, 0.5), radius: 10 });
      g.text('src/app/*.go', bx + 18, by - 32, { size: 20, font: 'mono', color: C.ink });
      g.chip('.agit/  ~2 KB', bx + bw - 18, by - 39, { color: C.blue, size: 18, align: 'right' });
    });
  },
});

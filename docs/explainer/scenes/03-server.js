// Scene 3: a capable server and thin clients that call APIs.
AGX.scene({
  id: 'server',
  order: 3,
  title: 'A smart server, thin clients',
  duration: 19,
  captions: [
    { at: 0, text: 'Commits, merges, rebases and lands all run on the server, so there is nothing to push.' },
    { at: 6.2, text: 'History, diffs, blame and file reads are API calls, so many jobs never need a checkout.' },
    { at: 12, text: 'Want files on disk? A working copy is just those files plus a small .agit directory, no history.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('The server does the work', { alpha: g.vis(0) });

    // When each server operation lights up: [start, end] (end null = until
    // the scene ends). Each one starts when the matching request arrives.
    const litSpans = {
      commit: [[3.4, 5.2], [14.8, 16.3]],
      land: [[16.1, null]],
      log: [[7.5, 9.1]],
      file: [[9.7, 11.6]],
    };
    const litOf = (name) => (litSpans[name] || []).reduce((m, [a, b]) => Math.max(m, g.vis(a, b, 0.25)), 0);
    const pulseOf = (name) => (litSpans[name] || []).reduce((m, [a]) => Math.max(m, g.at(a, 0.5, 'pulse')), 0);

    // Server panel.
    const px = 1110, py = 300, pw = 680, ph = 580;
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
    // A server operation chip; when its request arrives it fills solid,
    // glows and pulses once.
    const opChip = (label, x, y, color) => {
      const lit = litOf(label), pulse = pulseOf(label);
      const size = 24, h = Math.round(size * 1.6);
      const w = g.measure(label, { size, font: 'mono', weight: 500 }) + Math.round(size * 0.6) * 2;
      g.transform({ scale: 1 + 0.12 * pulse, cx: x + w / 2, cy: y }, () => {
        if (lit > 0.01) {
          g.box(x, y - h / 2, w, h, { radius: h / 2, fill: g.rgba(color, 0.9 * lit), shadow: g.rgba(color, 0.7 * lit), shadowBlur: 28 });
        }
        g.chip(label, x, y, {
          color, size,
          fill: g.rgba(color, 0.14 + 0.72 * lit),
          stroke: g.rgba(color, 0.7 + 0.3 * lit),
          textColor: g.mix(color, C.bg, lit),
        });
      });
    };
    writes.forEach((w, i) => {
      g.alpha(g.stagger(i, 0.8, 0.12, 0.4), () => opChip(w, px + 40, rowY(i), C.teal));
    });
    reads.forEach((r, i) => {
      g.alpha(g.stagger(i, 1.2, 0.12, 0.4), () => opChip(r, px + 370, rowY(i), C.violet));
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

    // A working copy belongs to the developer: files plus .agit/, no history.
    g.alpha(g.vis(12.2), () => {
      const bx = 370, by = 822, bw = 600, bh = 56;
      g.box(bx, by, bw, bh, { fill: C.panel, stroke: g.rgba(C.blue, 0.55), radius: 10 });
      g.text('working copy', bx + 18, by + 35, { size: 18, color: C.muted });
      g.text('src/app/*.go', bx + 150, by + 35, { size: 20, font: 'mono', color: C.ink });
      g.chip('.agit/ · no history', bx + bw - 16, by + bh / 2, { color: C.blue, size: 18, align: 'right' });
    });

    // Lanes and the calls travelling along them. A call's label rides with
    // its packet but parks left of the server card so it never covers it.
    const x0 = cx + 100, x1 = px - 20;
    const lane = (y, start, color) => {
      g.line(x0, y, x1, y, { color: g.rgba(color, 0.3), width: 2, dash: [5, 9], progress: g.at(start, 0.6) });
    };
    const call = (y, start, dur, label, color) => {
      const u = g.at(start, dur, 'inOut');
      if (u <= 0 || u >= 1) return;
      const pts = [[x0, y], [x1, y]];
      g.packet(pts, u, { color });
      const [x] = g.pointAt(pts, u);
      const w = g.measure(label, { size: 20, font: 'mono', weight: 500 }) + 24;
      g.chip(label, Math.min(x + 16, px - 30 - w), y - 30, { size: 20, color });
    };
    // The reply (an ID or file bytes) travelling back to the client.
    const reply = (y, start, dy) => {
      g.packet([[x1, y + dy], [x0, y + dy]], g.at(start, 0.9, 'inOut'), { color: C.amber, r: 7 });
    };

    // Agent: a commit uploads the blobs the server lacks, then the changeset.
    lane(380, 1.0, C.teal);
    call(380, 1.4, 0.8, 'POST /objects', C.amber);
    call(380, 2.3, 1.1, 'POST /changesets', C.teal);
    reply(380, 3.7, 16);

    // CI job: reads answered by the server, no checkout.
    lane(590, 6.2, C.violet);
    call(590, 6.5, 1.0, 'GET /log', C.violet);
    reply(590, 7.6, 16);
    call(590, 8.6, 1.1, 'GET /file/main?path=…', C.violet);
    reply(590, 10.0, 16);

    // Developer: commit from the working copy, then land it.
    lane(800, 12.4, C.teal);
    call(800, 12.8, 0.8, 'POST /objects', C.amber);
    call(800, 13.7, 1.1, 'POST /changesets', C.teal);
    call(800, 15.0, 1.1, 'POST /land', C.teal);
    reply(800, 16.4, -14);

    // Annotations under each client lane.
    g.alpha(g.vis(4.2), () => {
      g.chip('commit · no push', cx + 120, 450, { color: C.teal, size: 20 });
    });
    g.alpha(g.vis(10.2), () => {
      g.chip('no checkout needed', cx + 120, 660, { color: C.violet, size: 20 });
    });
  },
});

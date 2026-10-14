// Scene 6: lock-free changeset inserts, a write-ahead log, and land as
// compare-and-swap with rebase-and-retry instead of a global queue.
AGX.scene({
  id: 'lockfree',
  order: 6,
  title: 'Lock-free landing',
  duration: 18,
  captions: [
    { at: 0, text: "Creating a changeset takes no locks. It's an idempotent, content-addressed insert, appended to a write-ahead log." },
    { at: 6, text: 'Refs like main are the only mutable state, and they move by compare-and-swap. When two agents land at once, one swap fails.' },
    { at: 12, text: 'The server rebases the losing stack onto the new tip and swaps again. There is no global merge queue, and only real conflicts stop a land.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('No locks, no merge queue', { alpha: g.vis(0) });

    // Agents on the left.
    const A = { x: 230, y: 420 }, B = { x: 230, y: 720 };
    g.alpha(g.vis(0.2), () => {
      g.node(A.x, A.y, { icon: 'agent', label: 'agent A', color: C.blue, w: 180, h: 150, labelFont: 'body', labelSize: 22 });
      g.node(B.x, B.y, { icon: 'agent', label: 'agent B', color: C.blue, w: 180, h: 150, labelFont: 'body', labelSize: 22 });
    });

    // Commit graph in the middle.
    const gy = 570;
    const M0 = { x: 520, y: gy }, M1 = { x: 660, y: gy };
    const a1 = { x: 820, y: 440 }, b1 = { x: 820, y: 700 }, b1r = { x: 990, y: 440 };
    const dot = (p, col, label, a, labelBelow) => g.alpha(a, () => {
      g.circle(p.x, p.y, 20, { fill: C.bg, stroke: col, lineWidth: 4 });
      g.text(label, p.x, p.y + (labelBelow ? 52 : -34), { size: 20, font: 'mono', color: col, align: 'center' });
    });
    g.alpha(g.vis(0.4), () => g.line(M0.x - 80, gy, M1.x, gy, { color: C.line, width: 4 }));
    dot(M0, C.teal, 'm0', g.vis(0.4), true);
    dot(M1, C.teal, 'm1', g.vis(0.5), true);

    // Changeset creation: packets from agents, then nodes appear.
    const make = (from, to, start) => {
      g.packet([[from.x + 90, from.y], [to.x - 24, to.y]], g.at(start, 0.9, 'inOut'), { color: C.amber, r: 8 });
    };
    make(A, a1, 1.0);
    make(B, b1, 1.4);
    const a1In = g.at(1.9, 0.4, 'outBack'), b1In = g.at(2.3, 0.4, 'outBack');
    g.line(M1.x, gy, a1.x, a1.y, { color: g.rgba(C.blue, 0.7), width: 3, progress: a1In });
    g.line(M1.x, gy, b1.x, b1.y, { color: g.rgba(C.blue, 0.7), width: 3, progress: b1In });
    dot(a1, C.blue, 'a1', a1In);
    dot(b1, C.blue, 'b1', b1In, true);

    // Rebased b1' appears after the failed swap.
    const rbIn = g.at(10.0, 0.6, 'outBack');
    g.line(a1.x, a1.y, b1r.x, b1r.y, { color: g.rgba(C.mint, 0.8), width: 3, progress: rbIn });
    g.alpha(rbIn, () => g.arrow(b1.x + 22, b1.y - 18, b1r.x - 14, b1r.y + 22, { color: g.rgba(C.mint, 0.6), width: 2, dash: [6, 8], bend: -40 }));
    dot(b1r, C.mint, "b1'", rbIn);

    // Main pointer: m1 -> a1 (t=7.3) -> b1' (t=13.2).
    const mainAt = t < 7.3 ? M1 : t < 13.2 ? a1 : b1r;
    const moveU = t < 7.3 ? 1 : t < 13.2 ? g.at(7.3, 0.5) : g.at(13.2, 0.5);
    const prev = t < 7.3 ? M1 : t < 13.2 ? M1 : a1;
    const mx = g.lerp(prev.x, mainAt.x, moveU), my = g.lerp(prev.y, mainAt.y, moveU);
    g.alpha(g.vis(0.6), () => {
      g.chip('main', mx, my - 72, { color: C.teal, align: 'center', size: 20 });
      g.line(mx, my - 56, mx, my - 24, { color: C.teal, width: 2 });
    });

    // Right column: ref, CAS log and write-ahead log.
    const rx = 1180;
    g.alpha(g.vis(0.3), () => {
      g.card(rx, 300, 630, 300, { title: 'ref main · compare-and-swap', color: C.teal });
    });
    const cas = [
      { at: 6.6, text: 'A: main m1 → a1', ok: true },
      { at: 7.0, text: 'B: main m1 → b1', ok: false, note: 'main moved' },
      { at: 9.2, text: "rebase b1 onto a1 → b1'", ok: null },
      { at: 12.6, text: "B: main a1 → b1'", ok: true },
    ];
    cas.forEach((c, i) => {
      const yy = 380 + i * 52;
      g.alpha(g.at(c.at, 0.35, 'out'), () => {
        const col = c.ok === true ? C.mint : c.ok === false ? C.coral : C.amber;
        g.text(c.text, rx + 30, yy, { size: 22, font: 'mono', color: C.ink });
        if (c.ok !== null) g.icon(c.ok ? 'check' : 'cross', rx + 500, yy - 8, 26, { color: col, stroke: 5 });
        if (c.note) g.text(c.note, rx + 530, yy, { size: 18, color: C.coral });
      });
    });
    // Land requests from both agents to the ref box, concurrently.
    [[A, 5.6], [B, 5.75]].forEach(([p, s]) => {
      const pts = g.curvePoints(p.x + 90, p.y + (p === A ? -40 : 40), rx - 10, 330, p === A ? -60 : 80);
      g.packet(pts, g.at(s, 1.1, 'inOut'), { color: C.teal, label: 'land', size: 17 });
    });
    // Shake the failed swap.
    if (t > 7.0 && t < 7.6) {
      const k = Math.sin((t - 7.0) * 60) * 6 * (1 - (t - 7.0) / 0.6);
      g.box(rx + k, 300, 630, 300, { stroke: g.rgba(C.coral, 0.8), lineWidth: 3, radius: 16 });
    }

    // Write-ahead log.
    g.alpha(g.vis(0.5), () => {
      g.card(rx, 630, 630, 250, { title: 'write-ahead log', color: C.amber });
    });
    const wal = [
      { at: 2.1, seq: 57, id: 'a1' },
      { at: 2.5, seq: 58, id: 'b1' },
      { at: 10.4, seq: 59, id: "b1'" },
    ];
    wal.forEach((e, i) => {
      g.alpha(g.at(e.at, 0.35, 'out'), () => {
        const yy = 712 + i * 50;
        g.text('#' + e.seq, rx + 30, yy, { size: 21, font: 'mono', color: C.amber });
        g.text(g.hash('wal-' + e.id, 12) + '…', rx + 110, yy, { size: 21, font: 'mono', color: C.ink });
        g.text(e.id, rx + 450, yy, { size: 21, font: 'mono', color: C.muted });
      });
    });
    g.alpha(g.vis(3.0, 5.8), () => {
      g.chip('insert is idempotent · no lock taken', 440, 840, { color: C.amber, size: 20 });
    });
    g.alpha(g.vis(14.4), () => {
      g.chip('no global lock', 360, 860, { color: C.mint, size: 20 });
      g.chip('retry on contention', 580, 860, { color: C.mint, size: 20 });
      g.chip('conflicts are reported, not hidden', 850, 860, { color: C.coral, size: 20 });
    });
  },
});

// Scene 6: lock-free changeset inserts, a write-ahead log, and land as
// compare-and-swap with rebase-and-retry instead of a global queue.
AGX.scene({
  id: 'lockfree',
  order: 6,
  title: 'Lock-free landing',
  duration: 21,
  captions: [
    { at: 0, text: 'Storing a changeset takes no lock: an idempotent, content-addressed insert, appended to a write-ahead log.' },
    { at: 7.0, text: 'Main moves only by compare-and-swap. If two agents land at once, one swap can fail.' },
    { at: 12.6, text: 'The server rebases the losing stack onto the new tip and swaps again. A lost race is retried; real conflicts are reported.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('No global lock, no merge queue', { alpha: g.vis(0) });

    // Beat times (scene-local seconds).
    const T = {
      dup: 4.1,        // agent A sends a1 again (idempotent insert)
      landA: 7.6, landB: 7.75,
      casA: 9.0,       // A's swap is typed; result at +0.4
      casB: 9.8,       // B's swap is typed; it fails at +0.4
      rebase: 12.8,    // rebase row; b1' appears at rbStart
      rbStart: 13.2,
      wal59: 13.7,
      dim: 14.0,       // b1 is superseded by b1'
      casB2: 14.5,     // B's second swap; result at +0.4
      chips: 16.6,
    };
    const TYPE = 0.4;
    const failAt = T.casB + TYPE;

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

    // Changeset creation: packets from agents, then nodes appear.
    const make = (from, to, start) => {
      g.packet([[from.x + 90, from.y], [to.x - 24, to.y]], g.at(start, 0.9, 'inOut'), { color: C.amber, r: 8 });
    };
    make(A, a1, 1.0);
    make(B, b1, 1.4);
    // Node alpha overshoots (outBack); edges use 'out', which is exactly 0
    // before the start, so no zero-length round caps are drawn early.
    const a1In = g.at(1.9, 0.4, 'outBack'), b1In = g.at(2.3, 0.4, 'outBack');
    const a1Edge = g.at(1.9, 0.4, 'out'), b1Edge = g.at(2.3, 0.4, 'out');
    const rbIn = g.at(T.rbStart, 0.6, 'outBack'), rbEdge = g.at(T.rbStart, 0.6, 'out');
    // b1 flashes coral when its swap fails, then dims once b1' replaces it.
    const bFlash = g.at(failAt, 1.2, 'pulse');
    const bKeep = 1 - 0.65 * g.at(T.dim, 0.6);

    // Edges first, so the hollow dots cover their ends.
    g.alpha(g.vis(0.4), () => g.line(M0.x - 80, gy, M1.x, gy, { color: C.line, width: 4 }));
    g.line(M1.x, gy, a1.x, a1.y, { color: g.rgba(C.blue, 0.7), width: 3, progress: a1Edge });
    g.line(M1.x, gy, b1.x, b1.y, { color: g.rgba(C.blue, 0.7), width: 3, progress: b1Edge, alpha: bKeep });
    g.line(a1.x, a1.y, b1r.x, b1r.y, { color: g.rgba(C.mint, 0.8), width: 3, progress: rbEdge });
    g.alpha(rbIn * bKeep, () => g.arrow(b1.x + 22, b1.y - 18, b1r.x - 14, b1r.y + 22, { color: g.rgba(C.mint, 0.6), width: 2, dash: [6, 8], bend: -40 }));

    dot(M0, C.teal, 'm0', g.vis(0.4), true);
    dot(M1, C.teal, 'm1', g.vis(0.5), true);
    dot(a1, C.blue, 'a1', a1In);
    // An opaque disc keeps the dimmed edge from showing through b1's ring.
    g.alpha(b1In, () => g.circle(b1.x, b1.y, 20, { fill: C.bg }));
    dot(b1, g.mix(C.blue, C.coral, bFlash), 'b1', b1In * bKeep, true);
    dot(b1r, C.mint, "b1'", rbIn);

    // Idempotent insert: A sends a1 again; it is already stored.
    make(A, a1, T.dup);
    const ring = g.at(T.dup + 0.9, 0.6, 'out');
    if (ring > 0 && ring < 1) {
      g.circle(a1.x, a1.y, 20 + 16 * ring, { stroke: g.rgba(C.amber, 0.8 * (1 - ring)), lineWidth: 3 });
    }
    g.alpha(g.vis(T.dup + 1.0, 6.5), () => {
      g.chip('already stored · no-op', a1.x + 36, a1.y, { color: C.amber, size: 18 });
    });

    // Main pointer: m1 -> a1 when A's swap succeeds, a1 -> b1' after B's retry.
    const moveA = T.casA + TYPE, moveB = T.casB2 + TYPE;
    const mainAt = t < moveA ? M1 : t < moveB ? a1 : b1r;
    const prev = t < moveB ? M1 : a1;
    const moveU = t < moveA ? 1 : t < moveB ? g.at(moveA, 0.35) : g.at(moveB, 0.4);
    const pulse = Math.max(g.at(moveA + 0.35, 0.45, 'pulse'), g.at(moveB + 0.4, 0.45, 'pulse'));
    const mx = g.lerp(prev.x, mainAt.x, moveU), my = g.lerp(prev.y, mainAt.y, moveU);
    g.alpha(g.vis(0.6), () => {
      g.line(mx, my - 84, mx, my - 54, { color: C.teal, width: 2 });
      g.transform({ scale: 1 + 0.14 * pulse, cx: mx, cy: my - 100 }, () => {
        g.chip('main', mx, my - 100, { color: C.teal, align: 'center', size: 20 });
      });
    });

    // Right column: ref, CAS log and write-ahead log.
    const rx = 1180;
    g.alpha(g.vis(0.3), () => {
      g.card(rx, 300, 630, 300, { title: 'ref main · compare-and-swap', color: C.teal });
    });
    // Failed swap: the card stroke cross-fades to coral, in place.
    const cardFlash = t >= failAt ? 1 - g.at(failAt, 0.8, 'linear') : 0;
    if (cardFlash > 0) g.box(rx, 300, 630, 300, { stroke: g.rgba(C.coral, 0.85 * cardFlash), lineWidth: 2.5, radius: 16 });

    const cas = [
      { at: T.casA, text: 'A: main m1 → a1', ok: true },
      { at: T.casB, text: 'B: main m1 → b1', ok: false, note: 'main moved' },
      { at: T.rebase, text: "rebase b1 onto a1 → b1'", ok: null },
      { at: T.casB2, text: "B: main a1 → b1'", ok: true },
    ];
    cas.forEach((c, i) => {
      const yy = 380 + i * 52;
      const res = c.at + TYPE;
      // The failed row shakes sideways inside the card.
      const k = c.ok === false && t > res && t < res + 0.6 ? Math.sin((t - res) * 60) * 6 * (1 - (t - res) / 0.6) : 0;
      g.alpha(g.at(c.at, 0.2, 'out'), () => {
        const col = c.ok === true ? C.mint : C.coral;
        g.text(g.typed(c.text, g.at(c.at, c.ok === null ? 0.5 : TYPE, 'linear')), rx + 30 + k, yy, { size: 22, font: 'mono', color: C.ink });
        g.alpha(g.at(res, 0.25, 'out'), () => {
          if (c.ok !== null) g.icon(c.ok ? 'check' : 'cross', rx + 420 + k, yy - 8, 26, { color: col, stroke: 5 });
          if (c.note) g.text(c.note, rx + 450 + k, yy, { size: 18, color: C.coral });
        });
      });
    });

    // Land requests from both agents, concurrently. Each ends at the card's
    // left edge beside its own row; B goes around the graph, below it.
    const quad = (p0, p1, p2, n) => Array.from({ length: n + 1 }, (_, i) => {
      const u = i / n;
      return [
        (1 - u) * (1 - u) * p0[0] + 2 * (1 - u) * u * p1[0] + u * u * p2[0],
        (1 - u) * (1 - u) * p0[1] + 2 * (1 - u) * u * p1[1] + u * u * p2[1],
      ];
    });
    const pathA = g.curvePoints(A.x + 90, A.y - 35, rx - 14, 372, -60);
    const pathB = [
      ...quad([B.x + 90, B.y + 50], [B.x + 130, 820], [B.x + 210, 820], 12),
      ...quad([1020, 820], [1155, 820], [rx - 14, 424], 24),
    ];
    const land = (pts, u) => {
      if (u <= 0 || u >= 1) return;
      g.packet(pts, u, { color: C.teal });
      const [x, y] = g.pointAt(pts, u);
      // Label trails up-left of the dot; it fades in once clear of the agent box.
      g.alpha(g.clamp01((u - 0.1) / 0.1), () => g.chip('land', x - 70, y - 22, { size: 17, color: C.teal }));
    };
    land(pathA, g.at(T.landA, 1.4, 'inOut'));
    land(pathB, g.at(T.landB, 1.7, 'inOut'));

    // Write-ahead log.
    g.alpha(g.vis(0.5), () => {
      g.card(rx, 630, 630, 250, { title: 'write-ahead log', color: C.amber });
    });
    const wal = [
      { at: 2.1, seq: 57, id: 'a1' },
      { at: 2.5, seq: 58, id: 'b1' },
      { at: T.wal59, seq: 59, id: "b1'" },
    ];
    wal.forEach((e, i) => {
      g.alpha(g.at(e.at, 0.35, 'out'), () => {
        const yy = 712 + i * 50;
        g.text('#' + e.seq, rx + 30, yy, { size: 21, font: 'mono', color: C.amber });
        g.text(g.hash('wal-' + e.id, 12) + '…', rx + 110, yy, { size: 21, font: 'mono', color: C.ink });
        g.text(e.id, rx + 450, yy, { size: 21, font: 'mono', color: C.muted });
      });
    });

    // Chip rows start at x 140, the left edge of the agent boxes.
    g.alpha(g.vis(3.0, 6.7), () => {
      g.chip('insert is idempotent · no lock taken', 140, 860, { color: C.amber, size: 20 });
    });
    const closing = [
      ['no global lock', C.mint],
      ['retry on contention', C.mint],
      ['conflicts are reported, not hidden', C.coral],
    ];
    let cx = 140;
    closing.forEach(([label, color], i) => {
      const w = g.measure(label, { size: 20, font: 'mono', weight: 500 }) + 24;
      g.alpha(g.stagger(i, T.chips, 0.9, 0.4, 'out'), () => g.chip(label, cx, 860, { color, size: 20 }));
      cx += w + 20;
    });
  },
});

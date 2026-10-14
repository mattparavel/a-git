// Scene 2: git's full-replica model versus always-online clients.
AGX.scene({
  id: 'problem',
  order: 2,
  title: 'The problem with full replicas',
  duration: 19,
  captions: [
    { at: 0, text: 'Git treats every clone as a complete offline replica, history and all.' },
    { at: 5, text: "That suits laptops on flaky networks. For thousands of agents and CI jobs, it's mostly waste." },
    { at: 11.2, text: 'Those clients are always online, and most need only a few files. So a-git moves the heavy lifting to the server.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Every clone copies everything', { alpha: g.vis(0, 11.2) });
    g.heading('a-git keeps history on the server', { alpha: g.vis(11.65) });

    // The origin repository on the left; a bus runs from its right edge
    // through the gap between the two rows of clients.
    const sx = 330, sy = 570;
    const busX = sx + 110, busY = 570;

    // Six clients in a 3x2 grid on the right.
    const icons = ['agent', 'ci', 'agent', 'ci', 'agent', 'laptop'];
    const names = ['agent-1', 'ci-build', 'agent-2', 'ci-test', 'agent-3', 'laptop'];
    const clients = [];
    for (let r = 0; r < 2; r++) {
      for (let c = 0; c < 3; c++) {
        const x = 1050 + c * 260, y = 420 + r * 300;
        // Each copy runs along the bus, then turns into its own node and
        // stops where the client's history stack rests (right of the icon).
        const rest = [x + 72, y - 8];
        const pts = [[busX, busY], [rest[0], busY], rest];
        const trunk = rest[0] - busX;
        const len = trunk + Math.abs(rest[1] - busY);
        clients.push({ x, y, r, rest, pts, trunk, len });
      }
    }
    clients.forEach((p, i) => {
      p.start = 1.1 + i * 0.48;
      p.u = g.at(p.start, 1.5, 'inOut');
      p.dist = p.u * p.len;
      p.dim = g.stagger(i, 7.4, 0.12, 0.6);
      p.shrink = g.stagger(i, 12.3, 0.08, 1.2);
    });

    // History stack centred at (cx, cy): six layers for all of history.
    // `low` dims the five older layers; `morph` turns the newest layer into
    // the mint bar for the few files a job actually needs.
    const stack = (cx, cy, low, morph) => {
      const s = 0.5, w = 92 * s, lh = 18 * s, step = 9 * s;
      const bottom = cy + (lh + 5 * step) / 2;
      g.alpha(low, () => {
        for (let i = 0; i < 5; i++) {
          g.box(cx - w / 2, bottom - lh - i * step, w, lh, {
            fill: g.rgba(C.amber, 0.18 + i * 0.06), stroke: g.rgba(C.amber, 0.8), radius: 2, lineWidth: 1.5,
          });
        }
      });
      const topY = bottom - lh - 5 * step;
      const h = g.lerp(lh, 14, morph > 0 ? g.ease.outBack(morph) : 0);
      const col = g.mix(C.amber, C.mint, morph);
      g.box(cx - w / 2, g.lerp(topY, cy - 7, morph), w, h, {
        fill: g.rgba(col, g.lerp(0.48, 1, morph)), stroke: g.rgba(col, 0.8), radius: g.lerp(2, 3, morph), lineWidth: 1.5,
      });
    };

    // --- Pass 1: connections (drawn first so nodes cover their ends).
    // Phase 2: the copy lines fade and a thin blue link shows each client
    // is online, with small violet reads flowing from the server.
    const linkU = g.at(11.5, 0.8, 'out');
    const copyFade = g.at(12.3, 1.0);
    g.alpha(linkU * 0.45, () => {
      g.line(busX, busY, clients[2].rest[0], busY, { color: C.blue, width: 2 });
      clients.forEach((p) => g.line(p.rest[0], busY, p.rest[0], p.rest[1], { color: C.blue, width: 2 }));
    });
    const reads = g.vis(13.6);
    if (reads > 0) {
      clients.forEach((p, i) => {
        const period = 2.6, local = t - 13.6 - i * 0.43;
        if (local < 0) return;
        const u = (local % period) / 1.5;
        if (u <= 0 || u >= 1) return;
        g.alpha(reads, () => g.packet(p.pts, u, { color: C.violet, r: 6 }));
      });
    }
    g.alpha(1 - copyFade, () => {
      const amber = { color: g.rgba(C.amber, 0.4), width: 2, dash: [6, 8] };
      // One shared trunk, as far as the furthest copy has travelled.
      const reach = Math.max(0, ...clients.map((p) => Math.min(p.dist, p.trunk)));
      if (reach > 0) g.line(busX, busY, busX + reach, busY, amber);
      clients.forEach((p) => {
        const down = p.dist - p.trunk;
        if (down > 0) g.line(p.rest[0], busY, p.rest[0], busY + Math.sign(p.rest[1] - busY) * down, amber);
      });
    });

    // --- Pass 2: client nodes.
    const offline = g.vis(5.3, 7.6);
    clients.forEach((p, i) => {
      g.alpha(g.at(0.5 + i * 0.12, 0.5, 'out'), () => {
        if (i === 5 && offline > 0) {
          g.alpha(offline, () => g.box(p.x - 105, p.y - 75, 210, 150, { fill: C.panel, radius: 18, shadow: g.rgba(C.blue, 0.5) }));
        }
        g.node(p.x, p.y, { icon: icons[i], label: names[i], color: C.blue, w: 210, h: 150, labelSize: 20 });
      });
    });

    // --- Pass 3: history stacks, travelling and resting. A travelling stack
    // ends exactly where the resting one is drawn, so the hand-off is seamless.
    clients.forEach((p) => {
      if (p.u <= 0) return;
      if (p.u < 1) {
        const [x, y] = g.pointAt(p.pts, p.u);
        stack(x, y, 1, 0);
        return;
      }
      const low = (1 - 0.75 * p.dim) * (1 - p.shrink);
      stack(p.rest[0], p.rest[1], low, p.shrink);
    });

    // --- Pass 4: the origin, drawn last so copies slide out from under it.
    // In phase 2 it turns teal: the a-git server that does the work.
    const serve = g.at(14.6, 1.0);
    g.alpha(g.vis(0.2), () => {
      if (serve > 0) {
        g.alpha(serve, () => g.box(sx - 110, sy - 95, 220, 190, { fill: C.panel, radius: 18, shadow: g.rgba(C.teal, 0.45) }));
      }
      g.node(sx, sy, { icon: 'server', color: g.mix(C.muted, C.teal, serve), w: 220, h: 190 });
      const ly = sy + 95 - 44;
      g.text('origin', sx, ly, { size: 24, weight: 600, align: 'center', font: 'mono', color: C.ink, alpha: 1 - serve });
      g.text('server', sx, ly, { size: 24, weight: 600, align: 'center', font: 'mono', color: C.ink, alpha: serve });
      g.text('full history', sx, sy + 95 - 18, { size: 17, align: 'center', color: C.muted });
    });
    const ops = ['commit', 'merge', 'rebase', 'land'];
    const opW = ops.map((s) => g.measure(s, { size: 18, font: 'mono', weight: 500 }) + 22);
    let ox = sx - (opW.reduce((a, b) => a + b, 0) + 10 * (ops.length - 1)) / 2;
    ops.forEach((s, i) => {
      const a = g.stagger(i, 15.4, 0.3, 0.45, 'out');
      g.alpha(a, () => g.chip(s, ox, sy + 140 + 8 * (1 - a), { color: C.teal, size: 18 }));
      ox += opW[i] + 10;
    });

    // --- Labels and counters.
    const copies = clients.filter((p) => p.u >= 1).length;
    g.alpha(g.vis(1.6, 12.4), () => {
      g.text('history copied', 110, 820, { size: 22, color: C.muted, spacing: 1 });
      const w = g.text(`${copies} × 100%`, 110, 868, { size: 48, font: 'mono', weight: 600, color: C.amber });
      g.alpha(g.vis(8.2), () => g.text('mostly unused', 110 + w + 28, 866, { size: 26, color: C.muted }));
    });
    g.alpha(g.vis(12.9), () => {
      g.text('what the jobs needed', 110, 820, { size: 22, color: C.muted, spacing: 1 });
      g.text('a few files each', 110, 868, { size: 48, font: 'mono', weight: 600, color: C.mint });
    });

    // A full replica is what lets a laptop work offline.
    g.alpha(offline, () => {
      g.chip('works offline', clients[5].x, 832, { color: C.blue, align: 'center', size: 18 });
    });
    // a-git's assumption is about the clients.
    g.alpha(g.vis(11.5), () => {
      g.chip('always online', 1310, 300, { color: C.blue, align: 'center', size: 20 });
    });
  },
});

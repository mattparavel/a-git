// Scene 2: git's full-replica model versus always-online clients.
AGX.scene({
  id: 'problem',
  order: 2,
  title: 'The problem with full replicas',
  duration: 16,
  captions: [
    { at: 0, text: 'Git treats every clone as a complete, offline replica of the repository, history and all.' },
    { at: 5.5, text: 'That made sense for laptops on unreliable networks. With thousands of agents and CI jobs checking out code all day, most of that copying is wasted.' },
    { at: 11, text: 'Those clients are always online, and most need only a few files. So a-git moves the heavy lifting to the server.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Every clone copies everything', { alpha: g.vis(0, 15.4) });

    // The origin repository on the left.
    const sx = 330, sy = 560;
    g.alpha(g.vis(0.2), () => {
      g.node(sx, sy, { icon: 'server', label: 'origin', sub: 'full history', color: C.muted, w: 220, h: 190 });
    });

    // Six clients in a 3x2 grid on the right.
    const clients = [];
    const icons = ['agent', 'ci', 'agent', 'ci', 'agent', 'laptop'];
    const names = ['agent-1', 'ci-build', 'agent-2', 'ci-test', 'agent-3', 'laptop'];
    for (let r = 0; r < 2; r++) {
      for (let c = 0; c < 3; c++) {
        clients.push({ x: 1050 + c * 260, y: 420 + r * 300 });
      }
    }

    // History stack: a pile of layers that represents every object ever.
    const stack = (x, y, scale, color, a) => {
      g.alpha(a, () => {
        for (let i = 0; i < 6; i++) {
          g.box(x - 46 * scale, y - 30 * scale - i * 9 * scale, 92 * scale, 18 * scale, {
            fill: g.rgba(color, 0.18 + i * 0.06), stroke: g.rgba(color, 0.8), radius: 4 * scale, lineWidth: 1.5,
          });
        }
      });
    };

    clients.forEach((p, i) => {
      const appear = g.at(0.6 + i * 0.15, 0.5, 'out');
      g.alpha(appear, () => {
        g.node(p.x, p.y, { icon: icons[i], label: names[i], color: C.blue, w: 200, h: 150, labelSize: 20 });
      });
      // Copy of the whole history flowing to each client.
      const start = 1.6 + i * 0.55;
      const pts = g.curvePoints(sx + 110, sy, p.x - 100, p.y, (i % 2 ? -1 : 1) * 40);
      const u = g.at(start, 1.6, 'inOut');
      g.path(pts, { color: g.rgba(C.amber, 0.35), width: 2, progress: u, dash: [6, 8] });
      if (u > 0 && u < 1) {
        const [x, y] = g.pointAt(pts, u);
        stack(x, y + 26, 0.55, C.amber, 1);
      }
      // Once arrived, the client holds a full copy (shrinks in phase 2).
      const arrived = g.at(start + 1.5, 0.3, 'out');
      const shrink = g.at(11.5, 1.6, 'inOut');
      if (arrived > 0) {
        g.alpha(arrived * (1 - shrink * 0.85), () => stack(p.x + 64, p.y - 30, 0.5, C.amber, 1));
        // What the job actually needed: one small sliver.
        g.alpha(shrink, () => {
          g.box(p.x + 64 - 23, p.y - 55, 46, 10, { fill: C.mint, radius: 3 });
        });
      }
    });

    // Counter of data copied.
    const copies = clients.filter((_, i) => t >= 1.6 + i * 0.55 + 1.6).length;
    g.alpha(g.vis(2, 11.2), () => {
      g.text('history copied', 110, 820, { size: 22, color: C.muted, spacing: 1 });
      g.text(`${copies} × 100%`, 110, 868, { size: 48, font: 'mono', weight: 600, color: C.amber });
    });
    g.alpha(g.vis(11.8), () => {
      g.text('what the jobs needed', 110, 820, { size: 22, color: C.muted, spacing: 1 });
      g.text('a few files each', 110, 868, { size: 48, font: 'mono', weight: 600, color: C.mint });
    });

    // Phase 2 note on the origin node.
    g.alpha(g.vis(7.2, 11), () => {
      g.chip('always online', sx, 430, { color: C.blue, align: 'center', size: 20 });
    });
  },
});

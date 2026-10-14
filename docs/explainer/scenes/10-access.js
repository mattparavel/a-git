// Scene 10: per-directory access control enforced even through shared caches.
AGX.scene({
  id: 'access',
  order: 10,
  title: 'Access control',
  duration: 14,
  captions: [
    { at: 0, text: 'Access control works per repository and per directory. The most specific rule wins.' },
    { at: 6, text: 'Caches ask the server before handing out restricted objects, so a secret cached on a shared daemon never reaches the wrong person.' },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Access control down to the directory', { alpha: g.vis(0) });

    // Directory listing with a protected folder.
    const lx = 110, ly = 300;
    g.alpha(g.vis(0.2), () => g.card(lx, ly, 640, 250, { title: 'repository mono', color: C.teal }));
    const dirs = [['src/', false], ['pay/', false], ['secrets/', true]];
    dirs.forEach(([d, locked], i) => {
      g.alpha(g.stagger(i, 0.5, 0.2, 0.4), () => {
        const yy = ly + 100 + i * 50;
        g.icon('folder', lx + 50, yy - 8, 30, { color: locked ? C.coral : C.muted, stroke: 8 });
        g.text(d, lx + 84, yy, { size: 24, font: 'mono', color: C.ink });
        if (locked) g.icon('lock', lx + 230, yy - 10, 26, { color: C.coral, stroke: 8 });
      });
    });

    // Rules table.
    g.alpha(g.vis(1.6), () => g.card(lx, 580, 640, 300, { title: 'policy', color: C.amber }));
    const rules = [
      ['group:eng', '/', 'write'],
      ['user:ci', '/', 'read'],
      ['user:carol', 'secrets/', 'write'],
    ];
    rules.forEach((r, i) => {
      const yy = 680 + i * 54;
      const lit = r[1] === 'secrets/' && t > 3.6;
      g.alpha(g.stagger(i, 2.0, 0.25, 0.4), () => {
        g.text(r[0], lx + 30, yy, { size: 22, font: 'mono', color: C.ink });
        g.text(r[1], lx + 290, yy, { size: 22, font: 'mono', color: lit ? C.coral : C.muted });
        g.text(r[2], lx + 470, yy, { size: 22, font: 'mono', color: C.amber });
      });
    });
    g.alpha(g.vis(3.6), () => {
      g.text('secrets/ has its own rule: only carol', lx + 30, 850, { size: 19, color: C.coral });
    });

    // Shared daemon with a cached secret, two users and the server.
    const D = { x: 1180, y: 560 }, S = { x: 1640, y: 560 };
    const bob = { x: 880, y: 380 }, carol = { x: 880, y: 760 };
    g.alpha(g.vis(4.0), () => {
      g.node(D.x, D.y, { icon: 'db', label: 'agitd', sub: 'shared machine', color: '#7FB8E8', w: 230, h: 200 });
      g.node(S.x, S.y, { icon: 'server', label: 'agit-server', sub: 'checks policy', color: C.teal, w: 250, h: 200 });
      g.line(D.x + 115, D.y, S.x - 125, D.y, { color: C.line, width: 3 });
      g.chip('5d1e… secrets/key', D.x, D.y + 140, { color: C.coral, align: 'center', size: 18 });
      g.text('cached for carol earlier', D.x, D.y + 182, { size: 18, color: C.muted, align: 'center' });
    });
    g.alpha(g.vis(4.4), () => {
      g.node(bob.x, bob.y, { icon: 'agent', label: 'bob', color: C.blue, w: 150, h: 130, labelFont: 'body', labelSize: 22 });
      g.node(carol.x, carol.y, { icon: 'agent', label: 'carol', color: C.blue, w: 150, h: 130, labelFont: 'body', labelSize: 22 });
    });

    // Bob asks for the blob by hash.
    const bobPath = [[bob.x + 75, bob.y], [D.x - 115, D.y - 50]];
    g.packet(bobPath, g.at(6.2, 0.9), { color: C.blue, label: 'GET 5d1e…', size: 17 });
    g.packet([[D.x + 115, D.y - 24], [S.x - 125, D.y - 24]], g.at(7.2, 0.7), { color: C.amber, label: 'may bob read this?', size: 17, dy: -30 });
    g.packet([[S.x - 125, D.y + 24], [D.x + 115, D.y + 24]], g.at(8.0, 0.6), { color: C.coral, label: 'no', size: 17, dy: 30 });
    g.alpha(g.vis(8.6), () => {
      g.icon('cross', bob.x + 130, bob.y - 10, 36, { color: C.coral, stroke: 5 });
      g.text('403 forbidden', bob.x + 160, bob.y + 4, { size: 21, font: 'mono', color: C.coral });
    });

    // Carol gets it straight from the daemon's disk.
    const carolPath = [[carol.x + 75, carol.y], [D.x - 115, D.y + 50]];
    g.packet(carolPath, g.at(10.0, 0.8), { color: C.blue, label: 'GET 5d1e…', size: 17, dy: 30 });
    g.packet(carolPath.slice().reverse(), g.at(11.0, 0.7), { color: C.amber });
    g.alpha(g.vis(11.6), () => {
      g.icon('check', carol.x + 130, carol.y - 10, 32, { color: C.mint, stroke: 5 });
      g.text('served from cache', carol.x + 160, carol.y + 4, { size: 21, font: 'mono', color: C.mint });
    });
  },
});

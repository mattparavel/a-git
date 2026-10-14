// Scene 10: per-directory access control enforced even through shared caches.
AGX.scene({
  id: 'access',
  order: 10,
  title: 'Access control',
  duration: 15,
  captions: [
    { at: 0, text: 'Access control works per repository and per directory. The most specific rule wins.' },
    { at: 5.6, text: 'Caches ask the server what each caller may read, so a secret cached on a shared daemon never reaches the wrong person.' },
  ],
  draw(g) {
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
      g.alpha(g.stagger(i, 2.0, 0.25, 0.4), () => {
        g.text(r[0], lx + 30, yy, { size: 22, font: 'mono', color: C.ink });
        g.text(r[1], lx + 290, yy, { size: 22, font: 'mono', color: r[1] === 'secrets/' ? g.mix(C.muted, C.coral, g.at(3.6, 0.4)) : C.muted });
        g.text(r[2], lx + 470, yy, { size: 22, font: 'mono', color: C.amber });
      });
    });
    g.alpha(g.vis(3.6), () => {
      g.text('secrets/ has its own rule: only carol', lx + 30, 850, { size: 19, color: C.coral });
    });

    // Shared daemon with a cached secret, two users and the server.
    const D = { x: 1180, y: 560 }, S = { x: 1680, y: 560 };
    const bob = { x: 880, y: 380 }, carol = { x: 880, y: 760 };
    const gapX = (D.x + 115 + S.x - 125) / 2; // centre of the daemon-server gap
    g.alpha(g.vis(5.4), () => {
      g.node(D.x, D.y, { icon: 'db', label: 'agitd', sub: 'shared machine', color: '#7FB8E8', w: 230, h: 200 });
      g.node(S.x, S.y, { icon: 'server', label: 'agit-server', sub: 'checks policy', color: C.teal, w: 250, h: 200 });
      g.line(D.x + 115, D.y, S.x - 125, D.y, { color: C.line, width: 3 });
      g.icon('lock', D.x - 112, D.y + 139, 24, { color: C.coral, stroke: 8 });
      g.chip('5d1e… secrets/key', D.x + 10, D.y + 140, { color: C.amber, align: 'center', size: 18 });
      g.text('fetched by carol earlier', D.x, D.y + 182, { size: 18, color: C.muted, align: 'center' });
    });
    g.alpha(g.vis(5.7), () => {
      g.line(bob.x + 75, bob.y, D.x - 115, D.y - 50, { color: C.line, width: 2 });
      g.line(carol.x + 75, carol.y, D.x - 115, D.y + 50, { color: C.line, width: 2 });
      g.node(bob.x, bob.y, { icon: 'agent', label: 'bob', color: C.blue, w: 150, h: 130, labelFont: 'body', labelSize: 22 });
      g.node(carol.x, carol.y, { icon: 'agent', label: 'carol', color: C.blue, w: 150, h: 130, labelFont: 'body', labelSize: 22 });
    });

    // The daemon asks the server about this caller, then answers.
    const ask = [[D.x + 115, D.y - 24], [S.x - 125, D.y - 24]];
    const reply = [[S.x - 125, D.y + 24], [D.x + 115, D.y + 24]];
    const check = (start, who, ok) => {
      g.packet(ask, g.at(start, 0.7), { color: C.violet });
      g.alpha(g.vis(start, start + 0.7, 0.2), () => g.chip(`may ${who} read this?`, gapX, D.y - 54, { align: 'center', size: 17, color: C.violet }));
      const r0 = start + 0.8, col = ok ? C.mint : C.coral;
      g.packet(reply, g.at(r0, 0.6), { color: col });
      g.alpha(g.vis(r0, r0 + 0.6, 0.2), () => g.chip(ok ? 'yes' : 'no', gapX, D.y + 54, { align: 'center', size: 17, color: col }));
    };

    // Bob asks for the blob by hash and is refused.
    const bobPath = [[bob.x + 75, bob.y], [D.x - 115, D.y - 50]];
    g.packet(bobPath, g.at(6.2, 0.9), { color: C.blue });
    g.alpha(g.vis(6.2, 7.0, 0.2), () => g.chip('GET 5d1e…', 975, 340, { size: 17, color: C.blue }));
    check(7.2, 'bob', false);
    g.packet(bobPath.slice().reverse(), g.at(8.6, 0.5), { color: C.coral });
    g.alpha(g.vis(9.1), () => {
      g.icon('cross', bob.x + 130, bob.y - 10, 36, { color: C.coral, stroke: 5 });
      g.text('403 forbidden', bob.x + 160, bob.y + 4, { size: 21, font: 'mono', color: C.coral });
    });

    // Carol is checked too, then served from the daemon's disk.
    const carolPath = [[carol.x + 75, carol.y], [D.x - 115, D.y + 50]];
    g.packet(carolPath, g.at(9.8, 0.8), { color: C.blue });
    g.alpha(g.vis(9.8, 10.5, 0.2), () => g.chip('GET 5d1e…', 975, 800, { size: 17, color: C.blue }));
    check(10.7, 'carol', true);
    g.packet(carolPath.slice().reverse(), g.at(12.2, 0.7), { color: C.amber });
    g.alpha(g.vis(12.9), () => {
      g.icon('check', carol.x + 130, carol.y + 32, 32, { color: C.mint, stroke: 5 });
      g.text('allowed · served from cache', carol.x + 160, carol.y + 46, { size: 21, font: 'mono', color: C.mint });
    });
  },
});

// Scene 7: daemon -> cache -> server, caching immutable objects at each tier.
AGX.scene({
  id: 'tiers',
  order: 7,
  title: 'Caching tiers',
  duration: 16,
  captions: [
    { at: 0, text: 'Requests flow through tiers: a daemon on each machine, a regional cache, then the authoritative server.' },
    { at: 6, text: 'Objects never change once written, so every tier can cache them forever. A repeat checkout is served from local disk.' },
    { at: 11, text: "Writes pass straight through. Each tier talks only to the tier directly above it, so failures can't spread sideways." },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Every tier caches everything', { alpha: g.vis(0) });

    const y = 520;
    const tiers = [
      { x: 230, label: 'agit', sub: 'CLI · agent · CI', icon: 'agent', color: C.blue },
      { x: 680, label: 'agitd', sub: 'per machine · disk', icon: 'db', color: '#7FB8E8' },
      { x: 1130, label: 'agit-cache', sub: 'regional · memory', icon: 'bolt', color: '#5FC3D2' },
      { x: 1580, label: 'agit-server', sub: 'authoritative', icon: 'server', color: C.teal },
    ];
    tiers.forEach((tr, i) => {
      g.alpha(g.stagger(i, 0.2, 0.25, 0.5), () => {
        g.node(tr.x, y, { icon: tr.icon, label: tr.label, sub: tr.sub, color: tr.color, w: 250, h: 210, labelSize: 26 });
      });
    });
    // Links with their transport.
    const transports = ['unix socket', 'http', 'http'];
    for (let i = 0; i < 3; i++) {
      const x1 = tiers[i].x + 125, x2 = tiers[i + 1].x - 125;
      g.alpha(g.vis(0.8 + i * 0.2), () => {
        g.line(x1, y, x2, y, { color: C.line, width: 3 });
        g.text(transports[i], (x1 + x2) / 2, y + 36, { size: 18, font: 'mono', color: C.dim, align: 'center' });
      });
    }

    // Cache contents and hit/miss counters under each caching tier.
    const stored = (i) => (t > 3.6 - i * 0.45 ? 1 : 0);
    const counters = {
      1: { hits: t > 7.4 ? 1 : 0, misses: t > 1.8 ? 1 : 0 },
      2: { hits: 0, misses: t > 2.5 ? 1 : 0 },
    };
    [1, 2].forEach((i) => {
      const tr = tiers[i];
      g.alpha(g.vis(1.2), () => {
        const c = counters[i];
        g.text(`hits ${c.hits}  misses ${c.misses}`, tr.x, y + 150, { size: 20, font: 'mono', color: C.muted, align: 'center' });
        if (stored(i)) g.chip('7c1e09d2', tr.x, y + 196, { color: C.amber, align: 'center', size: 18 });
      });
    });
    g.alpha(g.vis(1.2), () => {
      g.chip('7c1e09d2', tiers[3].x, y + 196, { color: C.amber, align: 'center', size: 18 });
      g.text('source of truth', tiers[3].x, y + 150, { size: 20, color: C.muted, align: 'center' });
    });

    // Request 1: miss at daemon and cache, served by the server.
    const lane = (i, dir, off) => {
      const a = tiers[i].x + (dir > 0 ? 125 : -125), b = tiers[i + dir].x + (dir > 0 ? -125 : 125);
      return [[a, y - off], [b, y - off]];
    };
    const reqLabel = 'GET object 7c1e…';
    g.packet(lane(0, 1, 40), g.at(1.0, 0.8), { color: C.blue, label: reqLabel, size: 17, dy: -26 });
    g.packet(lane(1, 1, 40), g.at(1.9, 0.8), { color: C.blue });
    g.packet(lane(2, 1, 40), g.at(2.6, 0.8), { color: C.blue });
    g.packet(lane(3, -1, -40), g.at(3.4, 0.6), { color: C.amber });
    g.packet(lane(2, -1, -40), g.at(3.9, 0.6), { color: C.amber });
    g.packet(lane(1, -1, -40), g.at(4.4, 0.6), { color: C.amber });
    const badge = (i, text, col, start, end) => g.alpha(g.vis(start, end, 0.25), () => {
      g.chip(text, tiers[i].x, y - 140, { color: col, align: 'center', size: 20 });
    });
    badge(1, 'miss', C.coral, 1.8, 5.2);
    badge(2, 'miss', C.coral, 2.5, 5.2);
    badge(3, 'served', C.teal, 3.3, 5.2);

    // Request 2: hit at the daemon.
    g.packet(lane(0, 1, 40), g.at(6.6, 0.8), { color: C.blue, label: reqLabel, size: 17, dy: -26 });
    g.packet(lane(1, -1, -40), g.at(7.5, 0.6), { color: C.amber });
    badge(1, 'hit · local disk', C.mint, 7.4, 10.6);
    if (t > 7.4 && t < 9) {
      g.box(tiers[1].x - 125, y - 105, 250, 210, { stroke: g.rgba(C.mint, 0.9 * (1 - g.at(8, 1))), lineWidth: 4, radius: 18 });
    }

    // Writes pass through to the server.
    g.packet(lane(0, 1, 40), g.at(11.2, 0.6), { color: C.teal, label: 'POST /land', size: 17, dy: -26 });
    g.packet(lane(1, 1, 40), g.at(11.8, 0.6), { color: C.teal });
    g.packet(lane(2, 1, 40), g.at(12.4, 0.6), { color: C.teal });
    badge(3, 'writes go to the server', C.teal, 12.9, 15.6);

    // Via header and the ordering rule.
    g.alpha(g.vis(13.4), () => {
      g.text('X-Agit-Via: daemon,cache', tiers[2].x, y - 186, { size: 20, font: 'mono', color: C.muted, align: 'center' });
      const pts = g.curvePoints(tiers[1].x, y + 230, tiers[3].x, y + 230, -70);
      g.path(pts, { color: g.rgba(C.coral, 0.75), width: 3, dash: [8, 8] });
      const [mx, my] = g.pointAt(pts, 0.5);
      g.icon('cross', mx, my, 30, { color: C.coral, stroke: 5 });
      g.text('a tier never skips the one above it', mx + 34, my + 8, { size: 20, color: C.coral });
    });
  },
});

// Scene 7: daemon -> cache -> server, caching immutable objects at each tier.
AGX.scene({
  id: 'tiers',
  order: 7,
  title: 'Caching tiers',
  duration: 19,
  captions: [
    { at: 0, text: 'Requests climb tiers: a daemon per machine, a regional cache, then the authoritative server.' },
    { at: 6, text: 'Objects never change, so tiers never invalidate them. Repeat checkouts get objects from local disk.' },
    { at: 12.4, text: "Writes pass through. Tiers refuse calls from peers or higher tiers, so failures can't spread sideways." },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Tiers cache immutable data', { alpha: g.vis(0) });

    const y = 520;
    const W = 240, H = 210, hw = W / 2;
    // Caching tiers are violet (they serve reads); blue is the client, teal the server.
    const tiers = [
      { x: 270, label: 'agit', sub: 'CLI · agent · CI', icon: 'agent', color: C.blue },
      { x: 730, label: 'agitd', sub: 'per machine · disk', icon: 'db', color: C.violet },
      { x: 1190, label: 'agit-cache', sub: 'regional · memory', icon: 'bolt', color: C.violet },
      { x: 1650, label: 'agit-server', sub: 'authoritative', icon: 'server', color: C.teal },
    ];
    const gapX = (i) => (tiers[i].x + tiers[i + 1].x) / 2;
    tiers.forEach((tr, i) => {
      g.alpha(g.stagger(i, 0.2, 0.25, 0.5), () => {
        g.node(tr.x, y, { icon: tr.icon, label: tr.label, sub: tr.sub, color: tr.color, w: W, h: H, labelSize: 26 });
      });
    });
    // Links with their transport (labels sit below the response lane).
    const transports = ['unix socket', 'http', 'http'];
    for (let i = 0; i < 3; i++) {
      const x1 = tiers[i].x + hw, x2 = tiers[i + 1].x - hw;
      g.alpha(g.vis(0.8 + i * 0.2), () => {
        g.line(x1, y, x2, y, { color: C.line, width: 3 });
        g.text(transports[i], (x1 + x2) / 2, y + 86, { size: 20, font: 'mono', color: C.muted, align: 'center' });
      });
    }

    // Timeline. Request 1 misses at the daemon and the cache; the server serves it.
    const T = {
      req1: 1.2, missD: 2.0, hop2: 2.1, missC: 2.9, hop3: 3.0, served: 3.8,
      back3: 3.9, storeC: 4.5, back2: 4.6, storeD: 5.2, back1: 5.3,
      tags: 6.3,
      req2: 7.4, hit2: 8.2, req3: 9.8, hit3: 10.6,
      w1: 12.8, w2: 13.6, w3: 14.4, landed: 15.1, rule: 15.6,
    };
    const hash = '7c1e09d2a4f3';

    // Cache contents and hit/miss counters under each caching tier.
    const storedAt = { 1: T.storeD, 2: T.storeC };
    const counters = {
      1: { hits: (t > T.hit2 ? 1 : 0) + (t > T.hit3 ? 1 : 0), misses: t > T.missD ? 1 : 0 },
      2: { hits: 0, misses: t > T.missC ? 1 : 0 },
    };
    const storedChip = (x, t0) => {
      const a = g.at(t0, 0.3, 'out');
      g.alpha(a, () => {
        g.transform({ scale: 0.6 + 0.4 * g.at(t0, 0.4, 'outBack'), cx: x, cy: y + 196 }, () => {
          g.chip(hash, x, y + 196, { color: C.amber, align: 'center', size: 18 });
        });
      });
    };
    [1, 2].forEach((i) => {
      const tr = tiers[i];
      g.alpha(g.vis(1.2), () => {
        const c = counters[i];
        g.text(`hits ${c.hits}  misses ${c.misses}`, tr.x, y + 150, { size: 20, font: 'mono', color: C.muted, align: 'center' });
      });
      storedChip(tr.x, storedAt[i]);
      // Immutability: a cached copy never needs invalidation.
      g.alpha(g.stagger(i - 1, T.tags, 0.3, 0.4, 'out') * (1 - g.at(12.0, 0.4, 'in')), () => {
        g.text('immutable · never invalidated', tr.x, y + 238, { size: 17, color: C.mint, align: 'center' });
      });
    });
    g.alpha(g.vis(1.2), () => {
      g.chip(hash, tiers[3].x, y + 196, { color: C.amber, align: 'center', size: 18 });
      g.text('source of truth', tiers[3].x, y + 150, { size: 20, color: C.muted, align: 'center' });
    });

    // Packets travel on lanes above (requests) and below (responses) the link.
    const lane = (i, dir, off) => {
      const a = tiers[i].x + (dir > 0 ? hw : -hw), b = tiers[i + dir].x + (dir > 0 ? -hw : hw);
      return [[a, y - off], [b, y - off]];
    };
    // A request label sits still over the gap while its packet is in flight.
    const hopLabel = (i, text, col, start, end) => g.alpha(g.vis(start, end, 0.2), () => {
      g.chip(text, gapX(i), y - 80, { color: col, align: 'center', size: 17 });
    });
    const badge = (i, text, col, start, end) => g.alpha(g.vis(start, end, 0.25), () => {
      g.chip(text, tiers[i].x, y - 140, { color: col, align: 'center', size: 20 });
    });

    const reqLabel = 'POST …/objects/get';
    g.packet(lane(0, 1, 40), g.at(T.req1, 0.8), { color: C.blue });
    hopLabel(0, reqLabel, C.blue, T.req1, T.missD + 0.1);
    g.packet(lane(1, 1, 40), g.at(T.hop2, 0.8), { color: C.blue });
    g.packet(lane(2, 1, 40), g.at(T.hop3, 0.8), { color: C.blue });
    g.packet(lane(3, -1, -40), g.at(T.back3, 0.6), { color: C.amber });
    g.packet(lane(2, -1, -40), g.at(T.back2, 0.6), { color: C.amber });
    g.packet(lane(1, -1, -40), g.at(T.back1, 0.6), { color: C.amber });
    badge(1, 'miss', C.coral, T.missD, 6.0);
    badge(2, 'miss', C.coral, T.missC, 6.0);
    badge(3, 'served', C.teal, T.served, 6.0);

    // Requests 2 and 3: repeat checkouts hit the daemon's disk cache.
    const hitOutline = (t0) => {
      const a = 0.9 * g.at(t0, 0.25, 'out') * (1 - g.at(t0 + 1.0, 0.8));
      if (a > 0.001) g.box(tiers[1].x - hw, y - H / 2, W, H, { stroke: g.rgba(C.mint, a), lineWidth: 4, radius: 18 });
    };
    [[T.req2, T.hit2], [T.req3, T.hit3]].forEach(([r, h]) => {
      g.packet(lane(0, 1, 40), g.at(r, 0.8), { color: C.blue });
      hopLabel(0, reqLabel, C.blue, r, h + 0.1);
      g.packet(lane(1, -1, -40), g.at(h + 0.1, 0.6), { color: C.amber });
      hitOutline(h);
    });
    badge(1, 'hit · local disk', C.mint, T.hit2, 12.0);

    // Writes pass through to the server; each hop appends itself to X-Agit-Via.
    g.packet(lane(0, 1, 40), g.at(T.w1, 0.7), { color: C.teal });
    hopLabel(0, 'POST …/land', C.teal, T.w1, T.w2);
    g.packet(lane(1, 1, 40), g.at(T.w2, 0.7), { color: C.teal });
    g.packet(lane(2, 1, 40), g.at(T.w3, 0.7), { color: C.teal });
    const via = (i, text, start, end) => g.alpha(g.vis(start, end, 0.3), () => {
      const x = gapX(i);
      g.line(x, y - 182, x, y - 58, { color: g.rgba(C.muted, 0.45), width: 2, dash: [4, 6] });
      g.chip(text, x, y - 200, { color: C.muted, align: 'center', size: 18 });
    });
    via(1, 'X-Agit-Via: daemon', T.w2, T.w3 + 0.5);
    via(2, 'X-Agit-Via: daemon,cache', T.w3);
    badge(3, 'writes go to the server', C.teal, T.landed);

    // The ordering rule CheckVia enforces: no calls from a peer or a higher tier.
    const pts = g.curvePoints(tiers[2].x, y + 240, tiers[1].x, y + 240, -70);
    const grow = g.at(T.rule, 0.8);
    g.alpha(g.vis(T.rule, null, 0.2), () => {
      g.path(pts, { color: g.rgba(C.coral, 0.8), width: 3, dash: [8, 8], progress: grow, head: true, headSize: 16 });
    });
    const [mx, my] = g.pointAt(pts, 0.5);
    g.alpha(g.vis(T.rule + 0.5), () => {
      g.icon('cross', mx, my, 30, { color: C.coral, stroke: 5 });
      g.text('refused: calls from the same or a higher tier', mx, my + 50, { size: 20, color: C.coral, align: 'center' });
    });
  },
});

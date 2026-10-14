// Beginner 4: the big idea — keep history on the server and ask for what you need.
AGX.scene({
  id: 'idea',
  order: 4,
  title: 'The big idea',
  duration: 25,
  captions: [
    { at: 0, text: 'a-git starts from a simple observation: these machines are always online. So the history can stay in one place, on the server.' },
    { at: 9.5, text: 'Instead of copying everything, you ask the server for just what you need: a file, recent changes, or who changed a line.' },
    { at: 18.4, text: 'Like a library: you borrow the books you need, and the library keeps the rest.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Keep the history in one place', { alpha: g.vis(0) });

    // The server, holding the full history like a library holds its shelves.
    const sx = 1420, sy = 560;
    const left = sx - 300;
    // Three books that the questions below are about; they light up at the end.
    const picked = { '0-2': true, '1-7': true, '2-4': true };
    const lift = g.at(19.0, 0.6, 'outBack');
    g.alpha(g.vis(0.4), () => {
      g.box(left, sy - 230, 600, 460, { fill: C.panel, stroke: g.rgba(C.teal, 0.7), radius: 20, shadow: g.rgba(C.teal, 0.3) });
      g.icon('server', sx - 240, sy - 170, 56, { color: C.teal });
      g.text('a-git server', sx - 190, sy - 160, { size: 32, font: 'display', weight: 700, color: C.ink });
      g.text('keeps the full history', sx - 190, sy - 122, { size: 22, color: C.muted, alpha: g.at(5.4, 0.5) });
      // Shelves of history, filled as the caption says the history stays here.
      for (let r = 0; r < 3; r++) {
        const y = sy - 50 + r * 90;
        g.line(sx - 250, y + 34, sx + 250, y + 34, { color: C.line, width: 3 });
        for (let b = 0; b < 12; b++) {
          const a = g.at(5.4 + (r * 12 + b) * 0.03, 0.35, 'out');
          if (a <= 0) continue;
          const h = 46 + Math.floor(g.rand('shelf' + r + b) * 20);
          const col = [C.amber, C.teal, C.blue, C.violet][(r + b) % 4];
          const pick = picked[r + '-' + b];
          const dy = pick ? -12 * lift : 0;
          g.alpha(a, () => {
            g.box(sx - 240 + b * 40, y + 34 - h + dy, 30, h, {
              fill: g.rgba(pick && lift > 0 ? C.mint : col, pick ? 0.35 + 0.2 * lift : 0.35),
              stroke: g.rgba(pick && lift > 0 ? C.mint : col, 0.8), radius: 4, lineWidth: pick ? 1.5 + 1.5 * lift : 1.5,
              shadow: pick && lift > 0 ? g.rgba(C.mint, 0.5 * lift) : null, shadowBlur: 16,
            });
          });
        }
      }
    });

    // People and machines on the left.
    const asks = [
      { y: 400, who: 'You', icon: 'person', q: 'show me src/app.go', a: 'app.go', font: 'mono', at: 10.0 },
      { y: 590, who: 'build & test', icon: 'ci', q: 'recent changes to src/app?', a: '3 changes', font: 'body', at: 12.2 },
      { y: 780, who: 'AI helper', icon: 'agent', q: 'who last changed line 12?', a: 'Sam, in March', font: 'body', at: 14.4 },
    ];
    const px = 250;
    asks.forEach((k, i) => {
      g.alpha(g.stagger(i, 0.8, 0.3, 0.5), () => {
        if (k.icon === 'person') BK.person(g, px, k.y, { size: 0.75, color: C.blue });
        else BK.helper(g, px, k.y, { size: 0.8, icon: k.icon });
        g.text(k.who, px, k.y + 78, { size: 21, color: C.muted, align: 'center' });
      });
    });

    // The machines are always online: each has a live line to the server.
    asks.slice(1).forEach((k, i) => {
      g.alpha(g.vis(2.4 + i * 0.3, 9.2), () => {
        const w = g.chip('always online', 330, k.y, { color: C.blue, size: 22, font: 'body' });
        g.line(330 + w + 10, k.y, left - 12, k.y, { color: g.rgba(C.teal, 0.55), width: 2, dash: [6, 8], progress: g.at(3.0 + i * 0.3, 1.0) });
      });
    });

    // Questions go out; small answers come back.
    asks.forEach((k) => {
      g.alpha(g.vis(k.at), () => BK.bubble(g, 330, k.y - 22, k.q, { size: 26, color: C.ink }));
      const cw = g.measure(k.a, { size: 22, font: k.font, weight: 500 }) + 26;
      // Each lane ends at the server box's left edge; the lowest row ramps up
      // at the end so its dots land on the box instead of below its corner.
      const endY = Math.min(k.y + 32, sy + 190);
      const lane = [[330 + cw, k.y + 32], [left - 110, k.y + 32], [left - 10, endY]];
      g.packet(lane, g.at(k.at + 0.2, 0.8, 'inOut'), { color: C.blue, r: 7 });
      g.packet(lane.slice().reverse(), g.at(k.at + 1.0, 0.9, 'inOut'), { color: C.amber, r: 9 });
      g.alpha(g.vis(k.at + 1.9), () => g.chip(k.a, 330, k.y + 32, { color: C.amber, size: 22, font: k.font }));
    });

    // The library analogy, under the server.
    g.alpha(g.vis(18.8), () => {
      const label = 'borrow what you need, the rest stays here';
      const w = g.measure(label, { size: 24 }) + 52;
      const x0 = sx - w / 2;
      g.icon('book', x0 + 16, sy + 282, 36, { color: C.mint });
      g.text(label, x0 + 52, sy + 292, { size: 24, color: C.mint });
    });
  },
});

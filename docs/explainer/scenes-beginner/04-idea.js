// Beginner 4: the big idea — keep history on the server and ask for what you need.
AGX.scene({
  id: 'idea',
  order: 4,
  title: 'The big idea',
  duration: 24,
  captions: [
    { at: 0, text: 'a-git starts from a simple observation: these machines are always online. So the history can stay in one place, on the server.' },
    { at: 9.5, text: 'Instead of copying everything, you ask the server for what you need: one file, one folder, or the story behind a single line.' },
    { at: 18, text: 'Think of a library. You borrow the books you need, and the library keeps the archive.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Keep the history in one place', { alpha: g.vis(0) });

    // The server, holding the full history like a library holds its shelves.
    const sx = 1420, sy = 560;
    g.alpha(g.vis(0.4), () => {
      g.box(sx - 300, sy - 230, 600, 460, { fill: C.panel, stroke: g.rgba(C.teal, 0.7), radius: 20, shadow: g.rgba(C.teal, 0.3) });
      g.icon('server', sx - 240, sy - 170, 56, { color: C.teal });
      g.text('a-git server', sx - 190, sy - 160, { size: 32, font: 'display', weight: 700, color: C.ink });
      g.text('keeps the full history', sx - 190, sy - 122, { size: 22, color: C.muted });
      // Shelves of history.
      for (let r = 0; r < 3; r++) {
        const y = sy - 50 + r * 90;
        g.line(sx - 250, y + 34, sx + 250, y + 34, { color: C.line, width: 3 });
        for (let b = 0; b < 12; b++) {
          const h = 46 + Math.floor(g.rand('shelf' + r + b) * 20);
          const col = [C.amber, C.teal, C.blue, C.violet][(r + b) % 4];
          g.box(sx - 240 + b * 40, y + 34 - h, 30, h, { fill: g.rgba(col, 0.35), stroke: g.rgba(col, 0.8), radius: 4, lineWidth: 1.5 });
        }
      }
    });
    g.alpha(g.vis(2.4), () => {
      g.chip('always online', 560, 300, { color: C.blue, size: 24 });
    });

    // Clients ask questions and get small answers back.
    const asks = [
      { y: 410, who: 'You', icon: 'person', q: 'show me src/app.go', a: 'app.go', at: 10.0 },
      { y: 600, who: 'build', icon: 'ci', q: 'what changed this week?', a: '3 changes', at: 12.2 },
      { y: 790, who: 'AI helper', icon: 'agent', q: 'who wrote line 12?', a: 'Sam, in March', at: 14.4 },
    ];
    asks.forEach((k, i) => {
      g.alpha(g.stagger(i, 1.0, 0.3, 0.5), () => {
        if (k.icon === 'person') BK.person(g, 260, k.y, { size: 0.75, color: C.blue });
        else BK.helper(g, 260, k.y, { size: 0.8, icon: k.icon });
        g.text(k.who, 260, k.y + 78, { size: 21, color: C.muted, align: 'center' });
      });
      g.alpha(g.vis(k.at), () => BK.bubble(g, 340, k.y - 22, k.q, { size: 26, color: C.ink }));
      const back = g.at(k.at + 0.9, 1.0, 'inOut');
      g.packet([[sx - 310, k.y + 10], [720, k.y + 10]], back, { color: C.amber, r: 9 });
      g.alpha(g.vis(k.at + 1.9), () => g.chip(k.a, 340, k.y + 32, { color: C.amber, size: 22 }));
    });

    // The library analogy.
    g.alpha(g.vis(18.3), () => {
      g.icon('book', sx - 30, sy + 290, 40, { color: C.mint });
      g.text('borrow what you need, the archive stays put', sx + 4, sy + 300, { size: 24, color: C.mint });
    });
  },
});

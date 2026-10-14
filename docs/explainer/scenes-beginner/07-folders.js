// Beginner 7: a working copy with only the folders you need.
AGX.scene({
  id: 'folders',
  order: 7,
  title: 'Only the folders you need',
  duration: 23,
  captions: [
    { at: 0, text: "Sometimes you do want files on your own computer. a-git's clone can fetch just the folders you're working in." },
    { at: 8, text: 'Everything else stays on the server. Need another folder later? Add it, and only that folder is fetched.' },
    { at: 15.6, text: 'So your working copy stays small: just those files plus a tiny settings folder, with no history to download.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Take only the folders you need', { alpha: g.vis(0) });

    // The project on the server.
    const lx = 140, ly = 330;
    g.alpha(g.vis(0.3), () => g.card(lx, ly - 30, 560, 560, { title: 'project on the server', color: C.teal }));
    const folders = [
      ['src/app/', 'app'], ['src/lib/', 'lib'], ['docs/', null], ['design/', null], ['tools/', null], ['tests/', null],
    ];
    const taken = (key) => (key === 'app' && t > 3.4) || (key === 'lib' && t > 10.6);
    folders.forEach(([name, key], i) => {
      const y = ly + 70 + i * 74;
      g.alpha(g.stagger(i, 0.6, 0.15, 0.4), () => {
        const on = taken(key);
        BK.folder(g, lx + 50, y, name, { color: on ? C.mint : C.muted, note: on ? 'on your computer' : t > 8.4 ? 'stays on the server' : '', noteColor: on ? C.mint : C.dim });
      });
    });

    // The commands.
    g.alpha(g.vis(1.2), () => g.chip('agit clone demo work --sparse src/app', 820, 330, { color: C.blue, size: 24 }));
    g.alpha(g.vis(9.2), () => g.chip('agit sparse add src/lib', 820, 400, { color: C.blue, size: 24 }));

    // Your computer.
    const cx = 1240, cy = 470;
    g.alpha(g.vis(0.8), () => {
      g.box(cx - 40, cy - 10, 600, 420, { fill: C.panel, stroke: g.rgba(C.blue, 0.6), radius: 18 });
      g.icon('laptop', cx + 20, cy + 40, 52, { color: C.blue });
      g.text('your computer', cx + 70, cy + 52, { size: 28, font: 'display', weight: 700, color: C.ink });
    });
    // Folders arrive.
    const fly = (key, startAt, row) => {
      const src = folders.findIndex((f) => f[1] === key);
      const p0 = [lx + 380, ly + 62 + src * 74], p1 = [cx + 40, cy + 120 + row * 66];
      const u = g.at(startAt, 1.3, 'inOut');
      g.packet([p0, p1], u, { color: C.mint, r: 10 });
      g.alpha(g.at(startAt + 1.2, 0.3), () => BK.folder(g, cx + 30, cy + 130 + row * 66, key === 'app' ? 'src/app/' : 'src/lib/', { color: C.mint }));
    };
    fly('app', 2.2, 0);
    fly('lib', 9.6, 1);
    g.alpha(g.vis(15.8), () => {
      BK.folder(g, cx + 30, cy + 262, '.agit/', { color: C.blue, note: 'a few small settings files', noteColor: C.muted });
      g.text('history: on the server', cx + 30, cy + 340, { size: 23, color: C.teal });
    });
  },
});

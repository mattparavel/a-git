// Beginner 9: the everyday commands, with notes that map them to Git.
AGX.scene({
  id: 'daily',
  order: 9,
  title: 'Day to day',
  duration: 26,
  captions: [
    { at: 0, text: 'Day to day, it feels familiar. Clone the folders you need, edit, and check your changes with agit status.' },
    { at: 8.4, text: "agit commit saves your change on the server right away, so there's nothing to push. It isn't on main yet." },
    { at: 16.8, text: 'agit land puts it on main. If main moved on in the meantime, the server updates your change for you.' },
  ],
  draw(g, t) {
    const C = g.C;
    const tx = 110, ty = 170, tw = 1060, th = 712;
    g.alpha(g.vis(0), () => g.terminal(tx, ty, tw, th, { title: 'your terminal' }));

    const P = C.teal, O = C.muted;
    const script = [
      [0.6, [['$ ', P], ['agit clone demo work --sparse src/app', C.ink]], 1.4],
      [2.3, [['checked out demo at ', O], ['c4e51661c808', C.amber], [' (12 files) in 13ms', O]], 0],
      [4.4, [['$ ', P], ['agit status', C.ink]], 0.6],
      [5.4, [['M ', C.amber], ['src/app/app.go', C.ink]], 0],
      [5.6, [['A ', C.mint], ['src/app/helper.go', C.ink]], 0],
      [9.0, [['$ ', P], ['agit commit -m "Add a helper"', C.ink]], 1.0],
      [10.4, [['created changeset ', O], ['99e0ba9acf52', C.amber], [' (2 file(s)) in 16ms', O]], 0],
      [17.4, [['$ ', P], ['agit land', C.ink]], 0.5],
      [18.4, [['landed on main: ', O], ['5ea84d4734f0', C.amber], [' -> ', O], ['7d78055e2afa', C.amber]], 0],
      [18.4, [['  (rebased 1 changeset(s), 1 attempt(s), 7ms)', O]], 0],
    ];
    const lh = 54;
    const origin = { x: tx + 30, y: ty + 100 };
    script.forEach(([at, segs, typing], i) => {
      if (t < at) return;
      const u = typing ? g.at(at, typing, 'linear') : 1;
      g.code([segs], origin.x, origin.y + i * lh, { size: 26, progress: u });
    });

    // Notes in plain language, each next to the command it explains.
    const note = (y, at, title, body, col) => g.alpha(g.vis(at), () => {
      g.box(1220, y - 52, 590, 128, { fill: C.panel2, stroke: g.rgba(col, 0.6), radius: 16 });
      g.text(title, 1250, y - 8, { size: 28, font: 'display', weight: 700, color: col });
      g.text(body, 1250, y + 34, { size: 22, color: C.ink });
    });
    note(300, 2.6, 'clone', 'downloads just src/app', C.blue);
    note(470, 6.0, 'status', 'like git status', C.blue);
    note(640, 10.8, 'commit', 'saved on the server, nothing to push', C.amber);
    note(810, 19.0, 'land', 'onto main, updated for you if needed', C.teal);
  },
});

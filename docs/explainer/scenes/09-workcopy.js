// Scene 9: the everyday loop in a terminal, with the working copy's metadata.
AGX.scene({
  id: 'workcopy',
  order: 9,
  title: 'The working copy',
  duration: 18,
  captions: [
    { at: 0, text: "Here's the everyday loop. Clone materializes only the paths in your sparse profile." },
    { at: 5, text: 'Status compares each file against a small dirstate of sizes and timestamps, and rehashes only files that might have changed.' },
    { at: 9.6, text: 'Commit uploads just the blobs the server lacks and creates the changeset there. Land rebases when needed and moves main.' },
  ],
  draw(g, t) {
    const C = g.C;
    const tx = 110, ty = 170, tw = 1060, th = 712;
    g.alpha(g.vis(0), () => g.terminal(tx, ty, tw, th, { title: '~/work' }));

    const P = C.teal, O = C.muted;
    // [time the line starts, segments, typing duration (0 = instant output)]
    const script = [
      [0.4, [['$ ', P], ['agit clone demo work --sparse src/app', C.ink]], 1.4],
      [2.1, [['checked out demo at ', O], ['c4e51661c808', C.amber], [' (12 files) in 13ms', O]], 0],
      [3.0, [['$ ', P], ['vim src/app/app.go', C.ink]], 0.8],
      [4.6, [['$ ', P], ['agit status', C.ink]], 0.6],
      [5.6, [['M ', C.amber], ['src/app/app.go', C.ink]], 0],
      [5.8, [['A ', C.mint], ['src/app/helper.go', C.ink]], 0],
      [8.6, [['$ ', P], ['agit commit -m "app: return 2 and add a helper"', C.ink]], 1.2],
      [10.3, [['created changeset ', O], ['99e0ba9acf52', C.amber], [' (2 file(s)) in 16ms', O]], 0],
      [13.0, [['$ ', P], ['agit land', C.ink]], 0.5],
      [14.0, [['landed on main: ', O], ['5ea84d4734f0', C.amber], [' -> ', O], ['7d78055e2afa', C.amber]], 0],
      [14.0, [['  (rebased 1 changeset(s), 1 attempt(s))', O]], 0],
    ];
    const lh = 50;
    const origin = { x: tx + 30, y: ty + 96 };
    script.forEach(([at, segs, typing], i) => {
      if (t < at) return;
      const u = typing ? g.at(at, typing, 'linear') : 1;
      g.code([segs], origin.x, origin.y + i * lh, { size: 25, progress: u });
      // Blinking cursor at the end of the line being typed.
      const total = segs.reduce((n, s) => n + s[0].length, 0);
      if (typing && u < 1 && Math.floor(t * 3) % 2 === 0) {
        const shown = Math.floor(u * total);
        const w = g.measure('x', { size: 25, font: 'mono' }) * shown;
        g.box(origin.x + w + 2, origin.y + i * lh - 22, 13, 28, { fill: C.ink, radius: 2 });
      }
    });

    // Right panel: what lives in .agit/.
    const px = 1220, py = 300, pw = 590;
    g.alpha(g.vis(1.2), () => g.card(px, py, pw, 300, { title: '.agit/', color: C.blue }));
    const meta = [
      ['config.json', 'repo, remote, sparse profile'],
      ['state.json', 'base changeset and its root tree'],
      ['dirstate', 'mode, blob, size, mtime per file'],
    ];
    meta.forEach(([f, d], i) => {
      const yy = py + 104 + i * 66;
      const lit = (f === 'dirstate' && t > 4.8 && t < 8.6) || (f === 'state.json' && t > 10.4 && t < 12.6);
      g.alpha(g.stagger(i, 1.6, 0.25, 0.4), () => {
        g.text(f, px + 30, yy, { size: 22, font: 'mono', weight: 600, color: lit ? C.amber : C.ink });
        g.text(d, px + 30, yy + 28, { size: 18, color: C.muted });
      });
    });

    // Flow notes that track the terminal.
    const note = (start, end, lines, col) => g.alpha(g.vis(start, end), () => {
      g.box(px, 640, pw, 230, { fill: C.panel2, stroke: g.rgba(col, 0.5), radius: 14 });
      lines.forEach((l, i) => {
        g.text(l[0], px + 30, 694 + i * 54, { size: 21, font: l[2] ? 'mono' : 'body', color: l[1] || C.ink });
      });
    });
    note(4.8, 8.4, [
      ['stat matches dirstate → skip', C.mint],
      ['size or mtime differs → rehash', C.amber],
      ['no staging area: every file is tracked', C.muted],
    ], C.amber);
    note(8.8, 12.6, [
      ['1  which blobs are missing?', C.ink, true],
      ['2  upload only those', C.ink, true],
      ['3  create the changeset on the server', C.ink, true],
    ], C.teal);
    note(12.9, null, [
      ['server rebases onto the newest main', C.ink],
      ['then swaps the ref (compare-and-swap)', C.ink],
      ['your files update to the result', C.muted],
    ], C.mint);
  },
});

// Beginner 8: the everyday commands, with notes that map them to Git.
AGX.scene({
  id: 'daily',
  order: 8,
  title: 'Day to day',
  duration: 29,
  captions: [
    { at: 0, text: 'Day to day, it feels familiar. Clone the folders you need, edit, and check your changes with agit status.' },
    { at: 11.0, text: "agit commit saves your change on the server right away, so there's nothing to push. It isn't on main yet." },
    { at: 20.8, text: 'agit land puts it on main. If main moved on in the meantime, the server updates your change for you.' },
  ],
  draw(g, t, d) {
    const C = g.C;
    const P = C.teal, O = C.muted;

    // The terminal (the engine's title is 18 px, so draw a larger one).
    const tx = 110, ty = 170, tw = 880, th = 712;
    g.alpha(g.vis(0), () => {
      g.terminal(tx, ty, tw, th, {});
      g.text('your terminal', tx + tw / 2, ty + 30, { size: 20, color: C.muted, align: 'center', font: 'mono' });
    });

    // Rows: commands show their prompt at `p` and are typed from `at`;
    // output rows appear at `at`. Real a-git output (the land line wraps).
    const rows = [
      { cmd: 'agit clone demo work --sparse src/app', p: 0.3, at: 0.6, dur: 1.4 },
      { out: [['checked out demo at ', O], ['c4e51661c808', C.amber], [' (12 files) in 13ms', O]], at: 2.3 },
      { cmd: 'agit status', p: 2.6, at: 7.6, dur: 0.6 },
      { out: [['M ', C.amber], ['src/app/app.go', C.ink]], at: 8.6 },
      { out: [['A ', C.mint], ['src/app/helper.go', C.ink]], at: 8.8 },
      { cmd: 'agit commit -m "Add a helper"', p: 9.2, at: 11.4, dur: 1.0 },
      { out: [['created changeset ', O], ['99e0ba9acf52', C.amber], [' (2 file(s)) in 16ms', O]], at: 12.8 },
      { cmd: 'agit land', p: 13.1, at: 21.2, dur: 0.5 },
      { out: [['landed on main: ', O], ['5ea84d4734f0', C.amber], [' -> ', O], ['7d78055e2afa', C.amber]], at: 22.2 },
      { out: [['  (rebased 1 changeset(s), 1 attempt(s), 7ms)', O]], at: 22.2 },
      { cmd: '', p: 22.6 },
    ];
    const lh = 54, ox = tx + 30, oy = ty + 100, size = 26;

    // A soft band behind the rows of the step being explained.
    const bands = [[0, 1, 0.6, 5.6, C.blue], [2, 4, 7.6, 11.4, C.blue], [5, 6, 11.4, 21.2, C.amber], [7, 9, 21.2, d + 1, C.teal]];
    // It grows to cover each output row as that row appears.
    bands.forEach(([r0, r1, a, b, col]) => g.alpha(g.vis(a, b, 0.3), () => {
      let grow = 0;
      for (let r = r0 + 1; r <= r1; r++) grow += g.at(rows[r].at, 0.25);
      g.box(tx + 14, oy + r0 * lh - 36, tw - 28, grow * lh + 50, { fill: g.rgba(col, 0.1), radius: 8 });
    }));

    let cursorRow = -1;
    rows.forEach((r, i) => {
      if (t >= (r.p !== undefined ? r.p : r.at)) cursorRow = r.cmd !== undefined ? i : -1;
    });
    rows.forEach((r, i) => {
      const y = oy + i * lh;
      if (r.cmd === undefined) {
        if (t >= r.at) g.code([r.out], ox, y, { size });
        return;
      }
      if (t < r.p) return;
      g.text('$ ', ox, y, { size, font: 'mono', color: P });
      const px = ox + g.measure('$ ', { size, font: 'mono' });
      const typed = r.cmd ? g.typed(r.cmd, r.dur ? g.at(r.at, r.dur, 'linear') : 0) : '';
      g.text(typed, px, y, { size, font: 'mono', color: C.ink });
      if (i === cursorRow) {
        const typing = r.cmd && t >= r.at && t < r.at + r.dur;
        if (typing || Math.floor(t * 2) % 2 === 0) {
          g.box(px + g.measure(typed, { size, font: 'mono' }) + 2, y - 22, 13, 28, { fill: g.rgba(C.ink, 0.75), radius: 2 });
        }
      }
    });

    // Notes in plain words, one per step. The newest is bright; earlier
    // ones dim so the eye goes to what the caption is talking about.
    const nx = 1030, nw = 780, titleX = nx + 26, bodyX = nx + 158;
    const seg = (parts, y) => {
      let x = bodyX;
      parts.forEach(([s, col, mono]) => {
        x += g.text(s, x, y, { size: mono ? 22 : 23, font: mono ? 'mono' : 'body', color: col || C.ink });
      });
    };
    const notes = [
      { at: 2.7, title: 'clone', col: C.blue, lines: [
        [['downloads just the '], ['src/app', C.ink, 1], [' folder']],
        [['c4e51661c808', C.amber, 1], [' = a version ID, like a Git commit ID']],
      ] },
      { at: 5.6, title: 'edit', col: C.blue, lines: [
        [['you change '], ['app.go', C.amber, 1], [' and add '], ['helper.go', C.mint, 1]],
      ] },
      { at: 9.0, title: 'status', col: C.blue, lines: [
        [['like '], ['git', C.ink, 1], [' '], ['status', C.ink, 1]],
        [['M', C.amber, 1], [' = changed,   '], ['A', C.mint, 1], [' = new file']],
      ] },
      { at: 13.2, title: 'commit', col: C.amber, late: 16.4, lines: [
        [['saved on the server: nothing to push']],
        [['no '], ['git', C.ink, 1], [' '], ['add', C.ink, 1], [' step: it takes every changed file']],
        [['not on main yet: a draft, like a branch only you use']],
      ] },
      { at: 22.6, title: 'land', col: C.teal, lines: [
        [['puts your change on main for everyone']],
        [['rebased', O, 1], [' = moved on top of the newest main']],
      ] },
    ];
    const lineH = 36, gap = 30;
    let y = ty;
    notes.forEach((n, i) => {
      const grow = n.late ? g.at(n.late, 0.45) : 1;
      const shown = n.late ? n.lines.length - 1 + grow : n.lines.length;
      const h = 70 + (shown - 1) * lineH;
      const next = notes[i + 1];
      const a = g.vis(n.at) * (next ? 1 - 0.45 * g.at(next.at, 0.6) : 1);
      const top = y;
      g.alpha(a, () => {
        g.box(nx, top, nw, h, { fill: C.panel2, stroke: g.rgba(n.col, 0.6), radius: 16 });
        g.text(n.title, titleX, top + 45, { size: 28, font: 'display', weight: 700, color: n.col });
        n.lines.forEach((parts, j) => {
          const late = n.late && j === n.lines.length - 1;
          g.alpha(late ? g.at(n.late + 0.2, 0.4) : 1, () => seg(parts, top + 44 + j * lineH));
        });
      });
      y += h + gap;
    });
  },
});

// Scene 9: the everyday loop in a terminal, with the working copy's metadata.
AGX.scene({
  id: 'workcopy',
  order: 9,
  title: 'The working copy',
  duration: 21,
  captions: [
    { at: 0, text: 'Clone materializes only the paths in your sparse profile.' },
    { at: 4.8, text: "Status uses the dirstate's size, mtime and inode to rehash only files that may have changed." },
    { at: 11.0, text: 'Commit uploads just the blobs the server lacks and creates the changeset there.' },
    { at: 16.4, text: 'Land rebases onto the newest main when needed, then moves main.' },
  ],
  draw(g, t) {
    const C = g.C;
    const tx = 110, ty = 170, tw = 1060, th = 712;
    const cdDone = 3.0; // the title follows the shell's directory
    g.alpha(g.vis(0), () => g.terminal(tx, ty, tw, th, { title: t < cdDone ? '~' : '~/work' }));

    const P = C.teal, O = C.muted;
    // Commands are typed after their prompt appears; output lines appear at once.
    const rows = [
      { cmd: 'agit clone demo work --sparse src/app', at: 0.5, typing: 1.4 },
      { out: [['checked out demo at ', O], ['c4e51661c808', C.amber], [' (12 files) in 13ms', O]], at: 2.1 },
      { cmd: 'cd work', at: 2.6, typing: 0.4 },
      { cmd: 'vim src/app/app.go src/app/helper.go', at: 3.4, typing: 0.8 },
      { cmd: 'agit status', at: 4.6, typing: 0.6 },
      { out: [['M ', C.amber], ['src/app/app.go', C.ink]], at: 5.5 },
      { out: [['A ', C.mint], ['src/app/helper.go', C.ink]], at: 5.7 },
      { cmd: 'agit commit -m "app: return 2 and add a helper"', at: 10.6, typing: 1.2 },
      { out: [['created changeset ', O], ['99e0ba9acf52', C.amber], [' (2 file(s)) in 16ms', O]], at: 12.6 },
      { cmd: 'agit land', at: 16.0, typing: 0.5 },
      { out: [['landed on main: ', O], ['5ea84d4734f0', C.amber], [' -> ', O], ['7d78055e2afa', C.amber]], at: 17.3 },
      // The CLI prints this as one line; it wraps here to fit the terminal.
      { out: [['  (rebased 1 changeset(s), 1 attempt(s), 19ms)', O]], at: 17.3 },
      { cmd: '', at: Infinity, typing: 0 }, // idle prompt after land
    ];
    const size = 25, lh = 48;
    const ox = tx + 30, oy = ty + 92;
    const cw = g.measure('x', { size, font: 'mono' });
    let prevDone = 0;
    rows.forEach((r, i) => {
      const y = oy + i * lh;
      if (r.out) {
        if (t >= r.at) g.code([r.out], ox, y, { size });
        prevDone = r.at;
        return;
      }
      // The prompt shows up as soon as the previous command has finished.
      const promptAt = i === 0 ? 0.15 : prevDone + 0.2;
      prevDone = r.at + r.typing;
      if (t < promptAt) return;
      const u = g.at(r.at, r.typing, 'linear');
      g.text('$ ', ox, y, { size, font: 'mono', color: P });
      g.text(g.typed(r.cmd, u), ox + 2 * cw, y, { size, font: 'mono', color: C.ink });
      // Cursor: blinking while the prompt waits, solid while typing.
      const idle = t < r.at;
      const typing = !idle && u < 1;
      if ((idle && Math.floor((t - promptAt) * 2.5) % 2 === 0) || typing) {
        const n = 2 + Math.floor(u * r.cmd.length + 1e-6);
        g.box(ox + n * cw + 2, y - 22, 13, 28, { fill: C.ink, radius: 2 });
      }
    });

    // Right column, centered on the terminal: what lives in .agit/, then a note.
    const px = 1220, pw = 590, ch = 300, nh = 230;
    const py = ty + (th - (ch + 40 + nh)) / 2;
    const ny = py + ch + 40;
    g.alpha(g.vis(2.2), () => {
      g.card(px, py, pw, ch, { color: C.blue });
      // Drawn here because g.card upper-cases titles and the directory is `.agit`.
      g.text('.agit/', px + 22, py + 38, { size: 20, font: 'mono', weight: 600, color: C.blue });
    });
    const meta = [
      ['config.json', 'repo, remote, target ref, sparse profile', [[2.6, 4.4]]],
      ['state.json', 'base changeset and its root tree', [[12.8, 15.2], [17.9, 19.6]]],
      ['dirstate', 'mode, blob, size, mtime, inode per file', [[5.0, 10.6]]],
    ];
    meta.forEach(([f, d, spans], i) => {
      const yy = py + 104 + i * 66;
      const lit = spans.reduce((m, [a, b]) => Math.max(m, g.vis(a, b, 0.25)), 0);
      g.alpha(g.stagger(i, 2.4, 0.25, 0.4), () => {
        g.alpha(lit, () => g.box(px + 12, yy - 28, pw - 24, 68, { fill: g.rgba(C.blue, 0.16), radius: 8 }));
        g.text(f, px + 30, yy, { size: 22, font: 'mono', weight: 600, color: g.mix(C.ink, C.blue, lit) });
        g.text(d, px + 30, yy + 28, { size: 18, color: C.muted });
      });
    });

    // Flow notes that track the terminal. They share one slot, so each starts
    // only after the previous one has faded out. Each line is a list of
    // [text, color, font] segments plus the time it appears.
    const note = (start, end, col, lines) => g.alpha(g.vis(start, end), () => {
      g.box(px, ny, pw, nh, { fill: C.panel2, stroke: g.rgba(col, 0.5), radius: 14 });
      lines.forEach(([segs, at], i) => g.alpha(g.at(at, 0.35, 'out'), () => {
        let x = px + 30;
        for (const [s, color, font] of segs) {
          x += g.text(s, x, ny + 66 + i * 54, { size: 21, font: font || 'body', color: color || C.ink });
        }
      }));
    });
    note(2.2, 4.4, C.blue, [
      [[['src/app/', C.mint, 'mono'], [' → materialized', C.mint]], 2.4],
      [[['other paths → not fetched', C.muted]], 2.8],
      [[['the profile lives in ', C.muted], ['config.json', C.muted, 'mono']], 3.2],
    ]);
    note(4.8, 10.6, C.amber, [
      [[['stat matches, older than dirstate → skip', C.mint]], 5.0],
      [[['stat differs or too recent → rehash', C.amber]], 5.4],
      [[['no staging area: every non-ignored file counts', C.muted]], 5.9],
    ]);
    note(11.4, 16.0, C.teal, [
      [[['1  which blobs are missing?', C.ink, 'mono']], 11.8],
      [[['2  upload only those', C.ink, 'mono']], 12.2],
      [[['3  create the changeset on the server', C.ink, 'mono']], 12.6],
    ]);
    note(16.4, null, C.mint, [
      [[['if main moved, server rebases onto it', C.ink]], 16.9],
      [[['then compare-and-swaps main (retries)', C.ink]], 17.3],
      [[['your working copy updates to the result', C.muted]], 17.9],
    ]);
  },
});

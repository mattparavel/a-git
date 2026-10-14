// Beginner 5: a working copy with only the folders you need.
AGX.scene({
  id: 'folders',
  order: 5,
  title: 'Only the folders you need',
  duration: 25,
  captions: [
    { at: 0, text: "You still need files on your computer to edit. a-git's clone can fetch just the folders you work on." },
    { at: 8.4, text: 'Everything else stays on the server. Need another folder later? Add it, and only that folder is fetched.' },
    { at: 16.8, text: 'So your folder stays small: just those files plus a tiny .agit folder. No history is downloaded.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Take only the folders you need', { alpha: g.vis(0) });

    // When things happen (scene seconds).
    const T = { cmd: 1.4, work: 4.2, app: 4.4, stay: 8.8, legendOut: 10.0, add: 10.4, lib: 11.6, meta: 17.2, hist: 19.2, small: 21.4 };
    const FLY = 1.3;

    // ---------------------------------------------------------- the server
    const lx = 140, ly = 290, lw = 500, lh = 570;
    g.alpha(g.vis(0.3), () => g.card(lx, ly, lw, lh, { title: 'project on the server', color: C.teal, titleSize: 20 }));
    const folders = [
      ['src/app/', 'app'], ['src/lib/', 'lib'], ['docs/', null], ['design/', null], ['tools/', null], ['tests/', null],
    ];
    const rowY = (i) => ly + 94 + i * 74;
    const nameX = lx + 80;
    const nameW = (name) => g.measure(name, { size: 24, font: 'mono' });
    const landed = { app: T.app + FLY, lib: T.lib + FLY };
    folders.forEach(([name, key], i) => {
      const y = rowY(i);
      g.alpha(g.stagger(i, 0.6, 0.15, 0.4), () => {
        const on = key && t > landed[key];
        BK.folder(g, lx + 46, y, name, { color: on ? C.mint : C.muted });
        if (key) {
          g.alpha(g.vis(landed[key]), () => g.text('copied to you', nameX + nameW(name) + 18, y, { size: 21, color: C.mint }));
        }
      });
    });
    // One bracket for the folders that stay on the server; it lets go of
    // src/lib/ once that folder has been fetched.
    g.alpha(g.vis(T.stay), () => {
      const first = g.lerp(1, 2, g.at(landed.lib, 0.5));
      const bx = lx + 352;
      const top = rowY(first) - 30, bot = rowY(5) + 12;
      const col = g.rgba(C.teal, 0.8);
      g.line(bx, top, bx, bot, { color: col, width: 2.5 });
      g.line(bx - 12, top, bx, top, { color: col, width: 2.5 });
      g.line(bx - 12, bot, bx, bot, { color: col, width: 2.5 });
      const mid = (top + bot) / 2;
      g.text('stays on', bx + 18, mid - 6, { size: 22, color: C.teal });
      g.text('the server', bx + 18, mid + 22, { size: 22, color: C.teal });
    });
    g.alpha(g.vis(T.hist), () => g.text('+ all of its history', lx + 46, ly + lh - 32, { size: 22, color: C.teal }));

    // --------------------------------------------------------- the commands
    // The clone command, with each part explained in plain words underneath.
    const cmd = 'agit clone demo work --sparse src/app';
    const cx0 = 720, cy0 = 330, csize = 26;
    const mono = { size: csize, font: 'mono', weight: 500 };
    g.alpha(g.vis(T.cmd), () => g.chip(cmd, cx0, cy0, { color: C.blue, size: csize }));
    const textX = cx0 + Math.round(csize * 0.6);
    const part = (before, word) => {
      const x = textX + g.measure(before, mono);
      return { x, w: g.measure(word, mono) };
    };
    const legend = [
      { seg: part('agit clone ', 'demo'), label: 'the project', row: 0 },
      { seg: part('agit clone demo ', 'work'), label: 'your new folder', row: 1 },
      { seg: part('agit clone demo work ', '--sparse src/app'), label: 'only this folder', row: 0 },
    ];
    legend.forEach((l, i) => {
      g.alpha(g.stagger(i, T.cmd + 1.0, 0.6, 0.4) * (1 - g.at(T.legendOut, 0.4)), () => {
        const mx = l.seg.x + l.seg.w / 2;
        const uy = cy0 + 33;
        const ly2 = cy0 + 76 + l.row * 40;
        g.line(l.seg.x + 2, uy, l.seg.x + l.seg.w - 2, uy, { color: C.blue, width: 3 });
        g.line(mx, uy, mx, ly2 - 22, { color: g.rgba(C.blue, 0.6), width: 2 });
        g.text(l.label, mx, ly2, { size: 22, color: C.ink, align: 'center' });
      });
    });
    // Adding one more folder later.
    g.alpha(g.vis(T.add), () => g.chip('agit sparse add src/lib', cx0, 410, { color: C.blue, size: csize }));
    g.alpha(g.vis(T.add + 0.5), () => g.text('fetch one more folder', textX, 466, { size: 22, color: C.ink }));

    // -------------------------------------------------------- your computer
    const bx0 = 1080, by0 = 480, bw = 720, bh = 400;
    const wy = by0 + 124;            // the work/ folder row
    const childY = (r) => wy + 60 + r * 58;
    const childX = bx0 + 100;
    const glow = g.at(T.small, 0.6) * (1 - g.at(T.small + 1.6, 0.8));
    g.alpha(g.vis(0.8), () => {
      g.box(bx0, by0, bw, bh, {
        fill: C.panel, stroke: g.rgba(glow > 0 ? g.mix(C.blue, C.mint, glow) : C.blue, 0.6 + 0.3 * glow), radius: 18,
        lineWidth: 2 + glow, shadow: glow > 0.01 ? g.rgba(C.mint, 0.35 * glow) : null,
      });
      g.icon('laptop', bx0 + 50, by0 + 50, 52, { color: C.blue });
      g.text('your computer', bx0 + 100, by0 + 62, { size: 28, font: 'display', weight: 700, color: C.ink });
    });
    g.alpha(g.vis(T.small), () => g.chip('stays small', bx0 + bw - 30, by0 + 52, { color: C.mint, size: 22, align: 'right', font: 'body', weight: 600 }));
    // The new folder that clone creates, with the fetched folders inside it.
    g.alpha(g.vis(T.work), () => {
      BK.folder(g, bx0 + 50, wy, 'work/', { color: C.blue });
      g.text('your new folder', bx0 + 84 + nameW('work/') + 18, wy, { size: 21, color: C.muted });
    });
    const branch = (r, a) => g.alpha(a, () => {
      const col = g.rgba(C.line, 1);
      g.line(bx0 + 50, wy + 14, bx0 + 50, childY(r) - 8, { color: col, width: 2 });
      g.line(bx0 + 50, childY(r) - 8, childX - 26, childY(r) - 8, { color: col, width: 2 });
    });
    const fly = (key, startAt, r) => {
      const src = folders.findIndex((f) => f[1] === key);
      const name = folders[src][0];
      const p0 = [nameX + nameW(name) + 14, rowY(src) - 8], p1 = [childX - 10, childY(r) - 8];
      g.packet([p0, p1], g.at(startAt, FLY, 'inOut'), { color: C.mint, r: 10 });
      const a = g.at(startAt + FLY - 0.1, 0.3);
      branch(r, a);
      g.alpha(a, () => BK.folder(g, childX, childY(r), name, { color: C.mint }));
    };
    fly('app', T.app, 0);
    fly('lib', T.lib, 1);
    // The tiny .agit folder, and the history that stays behind.
    branch(2, g.vis(T.meta));
    g.alpha(g.vis(T.meta), () => {
      BK.folder(g, childX, childY(2), '.agit/', { color: C.blue });
      g.text('a few tiny files: which project, which version', childX + 34 + nameW('.agit/') + 18, childY(2), { size: 21, color: C.muted });
    });
    g.alpha(g.vis(T.hist), () => g.text('no history here: it stays on the server', bx0 + 50, by0 + bh - 26, { size: 22, color: C.teal }));
  },
});

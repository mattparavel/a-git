// Scene 8: the client-driven fetch protocol, sparse checkouts and resume.
AGX.scene({
  id: 'fetch',
  order: 8,
  title: 'Fetching only what is missing',
  duration: 16,
  captions: [
    { at: 0, text: "Fetching starts from a root tree. The client checks its local store and asks only for the objects it's missing, one directory level at a time." },
    { at: 6, text: 'A sparse checkout simply never asks for subtrees outside its profile. No special protocol is needed.' },
    { at: 11, text: "Every request names immutable objects by hash, so if a transfer breaks, the client retries just what didn't arrive." },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading("Ask only for what's missing", { alpha: g.vis(0) });
    g.alpha(g.vis(0.3), () => g.chip('agit clone demo --sparse src', 110, 312, { color: C.blue, size: 21 }));

    // Tree on the left.
    const N = {
      root: { label: '/', x: 560, y: 380, level: 0 },
      docs: { label: 'docs/', x: 260, y: 510, level: 1, parent: 'root', out: true },
      src: { label: 'src/', x: 560, y: 510, level: 1, parent: 'root' },
      tools: { label: 'tools/', x: 860, y: 510, level: 1, parent: 'root', out: true },
      app: { label: 'app/', x: 440, y: 640, level: 2, parent: 'src' },
      lib: { label: 'lib/', x: 690, y: 640, level: 2, parent: 'src', cached: true },
      guide: { label: 'guide.md', x: 260, y: 640, level: 2, parent: 'docs', out: true, file: true },
      build: { label: 'build.sh', x: 900, y: 640, level: 2, parent: 'tools', out: true, file: true },
      appgo: { label: 'app.go', x: 340, y: 770, level: 3, parent: 'app', file: true },
      helper: { label: 'helper.go', x: 540, y: 770, level: 3, parent: 'app', file: true, late: true },
      libgo: { label: 'lib.go', x: 690, y: 770, level: 3, parent: 'lib', file: true, cached: true },
    };
    // When each level is requested and arrives.
    const levelAt = [1.2, 3.0, 4.8, 6.6];
    const arrived = (n) => {
      if (n.out) return false;
      if (n.late) return t >= 13.4; // re-requested after the stream broke
      return t >= levelAt[n.level] + 0.8;
    };

    g.alpha(g.vis(0.4), () => {
      for (const n of Object.values(N)) {
        if (!n.parent) continue;
        const p = N[n.parent];
        g.line(p.x, p.y + 22, n.x, n.y - 22, { color: n.out ? C.lineSoft : C.line, width: 2 });
      }
      for (const [id, n] of Object.entries(N)) {
        const outFaded = n.out && t > 6.2;
        let col = C.muted, fill = C.panel;
        if (arrived(n)) { col = n.cached ? C.mint : C.amber; fill = g.rgba(col, 0.16); }
        if (n.cached && t >= levelAt[n.level] + 0.2) { col = C.mint; fill = g.rgba(C.mint, 0.16); }
        const bw = Math.max(92, g.measure(n.label, { size: 20, font: 'mono' }) + 32);
        g.alpha(outFaded ? 0.35 : 1, () => {
          g.box(n.x - bw / 2, n.y - 22, bw, 44, { fill, stroke: g.rgba(col, 0.85), radius: n.file ? 22 : 8, dash: n.out && t > 6.2 ? [6, 6] : null });
          g.text(n.label, n.x, n.y + 7, { size: 20, font: 'mono', color: C.ink, align: 'center' });
        });
      }
    });
    g.alpha(g.vis(6.4), () => {
      g.text('outside the profile: never requested', 260, 862, { size: 20, color: C.dim });
    });
    g.alpha(g.vis(4.0), () => {
      g.chip('already in local store', 820, 712, { color: C.mint, size: 18 });
    });

    // Request log on the right.
    const lx = 1080, ly = 300;
    g.alpha(g.vis(0.5), () => g.card(lx, ly, 730, 410, { title: 'requests', color: C.blue }));
    const reqs = [
      { at: levelAt[0], text: 'want root tree', n: '1 object' },
      { at: levelAt[1], text: 'want src/', n: '1 object', skip: 'docs/, tools/ skipped' },
      { at: levelAt[2], text: 'want app/', n: '1 object', skip: 'lib/ cached' },
      { at: levelAt[3], text: 'want app.go, helper.go', n: '2 blobs' },
      { at: 12.6, text: 'retry helper.go', n: '1 blob', retry: true },
    ];
    reqs.forEach((r, i) => {
      g.alpha(g.at(r.at, 0.35, 'out'), () => {
        const yy = ly + 90 + i * 62;
        g.text(String(i + 1), lx + 30, yy, { size: 21, font: 'mono', color: r.retry ? C.coral : C.amber });
        g.text(r.text, lx + 70, yy, { size: 22, font: 'mono', color: C.ink });
        g.text(r.n, lx + 700, yy, { size: 19, font: 'mono', color: C.muted, align: 'right' });
        if (r.skip) g.text(r.skip, lx + 70, yy + 26, { size: 17, color: C.dim });
      });
    });

    // The frame stream.
    const fy = 760, fx = 1080;
    g.alpha(g.vis(7.0), () => {
      g.text('response stream', fx, fy - 20, { size: 19, color: C.muted, spacing: 1 });
      const frames = [['id', 64], ['len', 54], ['bytes', 130], ['id', 64], ['len', 54], ['bytes', 170]];
      let x = fx;
      const broken = t > 10.6 && t < 12.6;
      frames.forEach(([lab, w], i) => {
        const a = g.at(7.2 + i * 0.35, 0.3, 'out');
        const lost = broken && i >= 3;
        g.alpha(a * (lost ? 0.25 : 1), () => {
          const col = lab === 'bytes' ? C.amber : C.blue;
          g.box(x, fy, w, 52, { fill: g.rgba(col, 0.15), stroke: g.rgba(col, 0.7), radius: 6 });
          g.text(lab, x + w / 2, fy + 34, { size: 18, font: 'mono', color: col, align: 'center' });
        });
        x += w + 8;
      });
      if (broken) {
        g.icon('cross', fx + 280, fy + 26, 34, { color: C.coral, stroke: 5 });
        g.text('connection reset', fx + 320, fy + 90, { size: 19, color: C.coral });
      }
      if (t >= 12.6) {
        g.text('resumed: only the missing object', fx, fy + 90, { size: 19, color: C.mint });
      }
    });
  },
});

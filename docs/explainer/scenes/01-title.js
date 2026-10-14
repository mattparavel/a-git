// Scene 1: title card with a growing changeset graph.
AGX.scene({
  id: 'title',
  order: 1,
  title: 'Introduction',
  duration: 9,
  captions: [
    { at: 0, text: 'This is a-git, a version control system built around one assumption: every client is online.' },
    { at: 4.6, text: "Here's how that assumption shapes everything, from the data model to the network." },
  ],
  draw(g, t) {
    const C = g.C;
    const cx = 960;

    // Wordmark and subtitle.
    const rise = 24 * (1 - g.at(0, 1.0, 'out'));
    g.alpha(g.at(0, 0.8, 'out'), () => {
      g.text('a-git', cx, 470 + rise, { size: 236, font: 'display', weight: 800, color: C.ink, align: 'center', spacing: -4 });
    });
    g.alpha(g.at(0.5, 0.8, 'out'), () => {
      g.text('source control for clients that are always online', cx, 556 + rise, { size: 44, color: C.muted, align: 'center' });
    });

    // A changeset graph drawn across the lower third: a main line with a
    // short side branch that merges back.
    const y = 700;
    const xs = [330, 500, 670, 840, 1010, 1180, 1350, 1520];
    const lineU = g.at(1.0, 2.6, 'inOut');
    g.line(xs[0], y, xs[xs.length - 1], y, { color: C.line, width: 4, progress: lineU });

    const branch = [[670, y], [760, 620], [920, 620], [1100, 620], [1180, y]];
    g.path(g.partial(branch, 1), { color: C.lineSoft, width: 4, progress: g.at(2.2, 1.6) });
    [[840, 620], [1010, 620]].forEach(([bx, by], i) => {
      const a = g.at(2.6 + i * 0.35, 0.4, 'outBack');
      if (a > 0) g.circle(bx, by, 13 * a, { fill: C.bg, stroke: C.blue, lineWidth: 4 });
    });

    xs.forEach((x, i) => {
      const a = g.at(1.1 + i * 0.28, 0.45, 'outBack');
      if (a <= 0) return;
      const tip = i === xs.length - 1;
      g.circle(x, y, (tip ? 17 : 13) * a, {
        fill: tip ? C.teal : C.bg, stroke: C.teal, lineWidth: 4,
        glow: tip ? g.rgba(C.teal, 0.7) : null,
      });
      g.alpha(g.at(1.4 + i * 0.28, 0.4), () => {
        g.text(g.hash('title-cs-' + i, 7), x, y + 52, { size: 19, font: 'mono', color: C.dim, align: 'center' });
      });
    });

    // The ref name pointing at the tip.
    g.alpha(g.at(3.6, 0.5, 'out'), () => {
      g.chip('main', 1520, 640, { color: C.teal, align: 'center', size: 22 });
      g.line(1520, 656, 1520, 678, { color: C.teal, width: 2 });
    });

    // Three short claims fade in under the graph.
    const claims = [
      ['commits run on the server', C.teal],
      ['refs move by compare-and-swap', C.amber],
      ['checkouts fetch only what you need', C.blue],
    ];
    const widths = claims.map(([s]) => g.measure(s, { size: 21, font: 'mono' }) + 30);
    const gap = 26;
    let x = cx - (widths.reduce((a, b) => a + b, 0) + gap * (claims.length - 1)) / 2;
    claims.forEach(([s, col], i) => {
      g.alpha(g.at(4.4 + i * 0.3, 0.5, 'out'), () => {
        g.chip(s, x, 828, { color: col, size: 21 });
      });
      x += widths[i] + gap;
    });
  },
});

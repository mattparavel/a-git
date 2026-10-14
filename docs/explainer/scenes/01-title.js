// Scene 1: title card with a growing changeset graph.
AGX.scene({
  id: 'title',
  order: 1,
  title: 'Introduction',
  duration: 11,
  captions: [
    { at: 0, text: 'This is a-git, a version control system built around one assumption: every client is online.' },
    { at: 6.0, text: "Here's how that assumption shapes everything, from the data model to the network." },
  ],
  draw(g, t) {
    const C = g.C;
    const cx = 960;

    // Wordmark and subtitle. The wordmark starts partly visible so the very
    // first frame of the video is never empty.
    const rise = 24 * (1 - g.at(0, 1.0, 'out'));
    g.alpha(0.35 + 0.65 * g.at(0, 0.6, 'out'), () => {
      g.text('a-git', cx, 470 + rise, { size: 236, font: 'display', weight: 800, color: C.ink, align: 'center', spacing: -4 });
    });
    g.alpha(g.at(0.5, 0.8, 'out'), () => {
      g.text('source control for clients that are always online', cx, 556 + rise, { size: 44, color: C.muted, align: 'center' });
    });

    // A changeset graph drawn across the lower third, centered on the
    // wordmark: the main line, plus a short stack of changesets based on an
    // older main changeset. The stack has no ref and is not landed (commits
    // never move refs), so it never joins back into main.
    const y = 700;
    const step = 170;
    const xs = [0, 1, 2, 3, 4, 5, 6, 7].map((i) => cx + (i - 3.5) * step);
    const tipX = xs[xs.length - 1];

    // The main line grows at a constant speed and reaches node i at
    // lineStart + i * nodeStep, which is exactly when that node pops in.
    const lineStart = 1.0, nodeStep = 0.3;
    const lineDur = nodeStep * (xs.length - 1);
    g.line(xs[0], y, tipX, y, { color: C.line, width: 4, progress: g.at(lineStart, lineDur, 'linear') });

    // The stack forks from node 2 and its nodes pop in as the path arrives.
    const by = 620;
    const stack = [[xs[2], y], [xs[2] + 90, by], [xs[4], by]];
    const stackNodes = [xs[3], xs[4]];
    const stackStart = 2.2, stackDur = 0.9;
    const stackLen = Math.hypot(90, y - by) + (xs[4] - xs[2] - 90);
    const stackU = g.at(stackStart, stackDur, 'linear');
    if (stackU > 0) g.path(stack, { color: g.rgba(C.blue, 0.6), width: 3, progress: stackU });
    stackNodes.forEach((bx) => {
      const dist = Math.hypot(90, y - by) + (bx - xs[2] - 90);
      const a = g.at(stackStart + stackDur * (dist / stackLen), 0.4, 'outBack');
      if (a > 0) g.circle(bx, by, 13 * a, { fill: C.bg, stroke: C.blue, lineWidth: 4 });
    });

    // Once 'main' points at it, the tip pings every 2 s (a ring expands and
    // fades while the glow swells), so the second half never freezes.
    const pingStart = 4.2, pingEvery = 2.0, pingDur = 1.4;
    const pingU = t < pingStart ? 1 : ((t - pingStart) % pingEvery) / pingDur;
    const swell = pingU < 1 ? Math.sin(Math.PI * pingU) : 0;
    const glowA = 0.6 + 0.35 * swell;
    if (pingU < 1) {
      g.circle(tipX, y, 17 + 21 * g.ease.out(pingU), { stroke: g.rgba(C.teal, 0.6 * (1 - pingU)), lineWidth: 2.5 });
    }
    xs.forEach((x, i) => {
      const a = g.at(lineStart + i * nodeStep, 0.45, 'outBack');
      if (a <= 0) return;
      const tip = i === xs.length - 1;
      g.circle(x, y, (tip ? 17 : 13) * a, {
        fill: tip ? C.teal : C.bg, stroke: C.teal, lineWidth: 4,
        glow: tip ? g.rgba(C.teal, glowA) : null,
        glowBlur: tip ? 24 + 16 * swell : undefined,
      });
      // Changeset IDs, shortened like the CLI does (12 hex characters).
      g.alpha(g.at(lineStart + nodeStep + i * nodeStep, 0.4), () => {
        g.text(g.hash('title-cs-' + i), x, y + 52, { size: 19, font: 'mono', color: g.rgba(C.amber, 0.8), align: 'center' });
      });
    });

    // The ref name pointing at the tip.
    g.alpha(g.at(3.6, 0.5, 'out'), () => {
      g.chip('main', tipX, 640, { color: C.teal, align: 'center', size: 22 });
      g.line(tipX, 656, tipX, 678, { color: C.teal, width: 2 });
    });

    // Three short claims fade in under the graph with the second caption.
    const claims = [
      ['commits run on the server', C.teal],
      ['refs move by compare-and-swap', C.teal],
      ['checkouts fetch only what you need', C.blue],
    ];
    const widths = claims.map(([s]) => g.measure(s, { size: 21, font: 'mono' }) + 30);
    const gap = 26;
    let x = cx - (widths.reduce((a, b) => a + b, 0) + gap * (claims.length - 1)) / 2;
    claims.forEach(([s, col], i) => {
      g.alpha(g.at(5.8 + i * 0.3, 0.5, 'out'), () => {
        g.chip(s, x, 828, { color: col, size: 21 });
      });
      x += widths[i] + gap;
    });
  },
});

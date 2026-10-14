// Beginner 7: landing on main, and what happens when main moved on first.
// Land rebases on the server and retries if someone else lands first
// (internal/repo/merge.go); merging is line by line (internal/textdiff).
AGX.scene({
  id: 'land',
  order: 7,
  title: 'Landing on main',
  duration: 25,
  captions: [
    { at: 0, text: "When your work is ready, you land it on main. Landing is a-git's way of pushing to main." },
    { at: 7, text: "In Git, if someone pushed first, you'd have to pull and try again. In a-git, the server puts your change on top for you." },
    { at: 16.4, text: 'Only if you both changed the same part of a file, like the same line, does a person need to decide.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Landing your work on main', { alpha: g.vis(0) });

    // The main line.
    const y = 520;
    const base = [380, 540, 700];
    const RX = 900, SX = 1120; // where Robin's and Sam's changes join main
    // Robin's change joins main at 4.0 s, Sam's at 13.6 s. Lines are drawn
    // before dots so that no line runs through a dot.
    const rDot = g.at(4.0, 0.5, 'outBack'), sDot = g.at(13.6, 0.5, 'outBack');
    const dot = (x, col, u) => {
      const r = 18 * Math.min(1, u);
      g.circle(x, y, r, { fill: C.bg });
      g.circle(x, y, r, { fill: g.rgba(col, 0.35), stroke: col, lineWidth: 5 });
    };
    // While caption 3 (a separate "what if") is up, the Robin and Sam demo
    // dims so the conflict card reads as its own case.
    const dim = 1 - 0.6 * g.at(16.6, 0.6);
    g.alpha(g.vis(0.3) * dim, () => {
      g.line(260, y, 1720, y, { color: C.lineSoft, width: 4, dash: [2, 12] });
      g.line(260, y, base[2], y, { color: C.teal, width: 6 });
      if (rDot > 0) g.line(base[2], y, g.lerp(base[2], RX, Math.min(1, rDot)), y, { color: C.teal, width: 6 });
      if (sDot > 0) g.line(RX, y, g.lerp(RX, SX, Math.min(1, sDot)), y, { color: C.teal, width: 6 });
      base.forEach((x) => g.circle(x, y, 18, { fill: C.bg, stroke: C.teal, lineWidth: 5 }));
      if (rDot > 0) dot(RX, C.blue, rDot);
      if (sDot > 0) dot(SX, C.violet, sDot);
      g.text('main', 260, y - 40, { size: 28, font: 'mono', weight: 600, color: C.teal });
    });

    const card = (x, cy, label, col, a) => g.alpha(a, () => {
      g.box(x - 110, cy - 32, 220, 64, { fill: g.rgba(col, 0.18), stroke: g.rgba(col, 0.9), radius: 12, lineWidth: 2 });
      g.text(label, x, cy + 9, { size: 24, weight: 600, color: C.ink, align: 'center' });
    });

    // 1. Robin lands alone: the change simply joins the end of main.
    const robin = { x: 330, y: 320 };
    g.alpha(g.vis(0.8) * dim, () => BK.person(g, robin.x, robin.y, { name: 'Robin', color: C.blue, size: 0.8 }));
    g.alpha(g.vis(1.8, 4.6), () => g.chip('agit land', 580, 398, { color: C.blue, size: 24, align: 'center' }));
    // Across first, then down, so the card never passes over the chip.
    const rAcross = g.at(2.6, 0.9, 'inOut'), rDown = g.at(3.4, 0.7, 'inOut');
    card(g.lerp(580, RX, rAcross), g.lerp(robin.y, y - 84, rDown), "Robin's change", C.blue, g.vis(1.2) * dim);
    g.alpha(g.vis(4.6, 6.8), () => g.chip('like a push to main', RX + 140, y - 84, { color: C.teal, font: 'body', size: 24 }));

    // 2. Sam's change was made before Robin landed. In Git, Sam's push would
    // be turned away; in a-git the server puts it on top of Robin's.
    const sam = { x: 330, y: 760 };
    g.alpha(g.vis(7.0) * dim, () => BK.person(g, sam.x, sam.y, { name: 'Sam', color: C.violet, size: 0.8 }));
    // A faint branch line: Sam started from main before Robin's change.
    g.line(base[2], y + 20, 640, sam.y - 34, { color: g.rgba(C.violet, 0.6), width: 3, dash: [6, 8], alpha: g.vis(7.6, 12.0) });
    const sMove = g.at(12.2, 1.6, 'inOut');
    card(g.lerp(580, SX, sMove), g.lerp(sam.y, y + 84, sMove), "Sam's change", C.violet, g.vis(7.2) * dim);
    g.alpha(g.vis(14.0, 16.2), () => g.chip('put on top for you', SX + 140, y + 84, { color: C.mint, font: 'body', size: 24 }));

    // The Git way, for comparison: extra steps for Sam.
    g.alpha(g.vis(7.6, 16.2) * (1 - 0.45 * g.at(12.0, 0.6)), () => {
      const bx = 1180, by = 280, bw = 580, bh = 170;
      g.box(bx, by, bw, bh, { fill: C.panel, stroke: g.rgba(C.muted, 0.5), radius: 16 });
      g.text('In Git, if someone pushed first:', bx + 28, by + 46, { size: 24, weight: 600, color: C.muted });
      const steps = [['push', C.coral, 8.4], ['pull', C.ink, 9.4], ['push again', C.ink, 10.4]];
      let x = bx + 28;
      const cyRow = by + 112;
      steps.forEach(([label, col, at], i) => {
        const a = g.at(at, 0.4);
        if (i > 0) {
          g.arrow(x + 12, cyRow, x + 48, cyRow, { color: C.muted, width: 2.5, headSize: 10, alpha: a });
          x += 62;
        }
        // A chip is its label plus 2 x 14 px padding at size 24.
        const w = g.measure(label, { size: 24, font: 'body', weight: 500 }) + 28;
        g.alpha(a, () => g.chip(label, x, cyRow, { color: col === C.ink ? C.muted : col, textColor: col, font: 'body', size: 24 }));
        x += w;
        if (i === 0) {
          g.alpha(a, () => g.icon('cross', x + 22, cyRow, 26, { color: C.coral, stroke: 8 }));
          x += 44;
        }
      });
    });

    // 3. A separate "what if": a person decides only when both changed the
    // same part of a file. Then Land stops with a conflict (RebaseStack
    // returns a ConflictError) and nothing lands. The card sits apart from the
    // dimmed main line, with a dashed border and "You / Teammate" labels, so
    // it does not read as Robin's and Sam's changes clashing.
    g.alpha(g.vis(17.4), () => {
      const bx = 1170, by = 270, bw = 590, bh = 206;
      g.card(bx, by, bw, bh, { title: 'What if you both changed one line?', titleSize: 22, color: C.coral, dash: [10, 8] });
      g.text('You:      price = 10', bx + 30, by + 88, { size: 26, font: 'mono', color: C.ink });
      g.text('Teammate: price = 12', bx + 30, by + 132, { size: 26, font: 'mono', color: C.ink });
      const lineW = g.measure('Teammate: price = 12', { size: 26, font: 'mono' });
      const kx = bx + 30 + lineW + 24;
      g.path([[kx, by + 66], [kx + 18, by + 66], [kx + 18, by + 126], [kx, by + 126]], { color: C.coral, width: 3, progress: g.at(20.4, 0.6) });
      const fw = g.text('a-git stops, and you decide', bx + 30, by + 180, { size: 24, weight: 600, color: C.coral, alpha: g.at(21.4, 0.5) });
      g.icon('person', bx + 30 + fw + 30, by + 170, 34, { color: C.coral, alpha: g.at(22.2, 0.5) });
    });
  },
});

// Beginner 1: welcome. Sets up what the viewer already knows.
AGX.scene({
  id: 'welcome',
  order: 1,
  title: 'Welcome',
  duration: 14,
  captions: [
    { at: 0, text: 'a-git is a tool for saving and sharing code, like Git. You already know the basics.' },
    { at: 7.4, text: "It keeps commits and branches, but changes where the work happens. Let's see how." },
  ],
  draw(g, t) {
    const C = g.C;
    const cx = 960;
    const rise = 24 * (1 - g.at(0, 1.0, 'out'));
    g.alpha(0.35 + 0.65 * g.at(0, 0.7, 'out'), () => {
      g.text('a-git', cx, 440 + rise, { size: 220, font: 'display', weight: 800, color: C.ink, align: 'center', spacing: -4 });
    });
    g.alpha(g.at(0.5, 0.8, 'out'), () => {
      g.text('a new take on version control', cx, 528 + rise, { size: 46, color: C.muted, align: 'center' });
    });

    // What you already know: three familiar words.
    const words = [['commit', C.amber], ['branch', C.teal], ['push & pull', C.blue]];
    g.alpha(g.vis(2.6), () => {
      g.text('you already know', cx, 650, { size: 26, color: C.muted, align: 'center', spacing: 1 });
    });
    const size = 30;
    const widths = words.map(([w]) => g.measure(w, { size, font: 'mono', weight: 500 }) + Math.round(size * 0.6) * 2);
    const gap = 34;
    let x = cx - (widths.reduce((a, b) => a + b, 0) + gap * (words.length - 1)) / 2;
    words.forEach(([w, col], i) => {
      // Commit and branch glow briefly when the caption says a-git keeps them.
      const u = i < 2 ? g.at(8.0 + i * 0.35, 1.4, 'linear') : 0;
      const keep = u > 0 && u < 1 ? Math.sin(Math.PI * u) : 0;
      if (keep > 0) {
        const h = Math.round(size * 1.6);
        g.box(x - 6, 712 - h / 2 - 6, widths[i] + 12, h + 12, {
          stroke: g.rgba(col, 0.7 * keep), lineWidth: 2, radius: h / 2 + 6, shadow: g.rgba(col, 0.5 * keep), shadowBlur: 20,
        });
      }
      g.alpha(g.at(3.0 + i * 0.45, 0.5, 'out'), () => g.chip(w, x, 712, { color: col, size }));
      x += widths[i] + gap;
    });

    // What changes: where the work happens.
    g.alpha(g.vis(9.4), () => {
      const lead = 'what changes: ';
      const tail = 'where the work happens';
      const o = { size: 32, weight: 600 };
      const lw = g.measure(lead, o), tw = g.measure(tail, o);
      const left = cx - (lw + tw) / 2;
      g.text(lead, left, 820, Object.assign({ color: C.muted }, o));
      g.text(tail, left + lw, 820, Object.assign({ color: C.teal }, o));
      // Underline the part that changes.
      g.line(left + lw, 836, left + lw + tw * g.at(10.0, 0.8, 'out'), 836, { color: g.rgba(C.teal, 0.8), width: 3 });
    });
  },
});

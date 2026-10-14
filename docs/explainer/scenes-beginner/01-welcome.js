// Beginner 1: welcome. Sets up what the viewer already knows.
AGX.scene({
  id: 'welcome',
  order: 1,
  title: 'Welcome',
  duration: 13,
  captions: [
    { at: 0, text: "If you've used Git, you know the basics: you save your work as commits and share them with your team." },
    { at: 6.8, text: "a-git keeps those ideas but changes where the work happens. Let's see how." },
  ],
  draw(g, t) {
    const C = g.C;
    const cx = 960;
    const rise = 24 * (1 - g.at(0, 1.0, 'out'));
    g.alpha(0.35 + 0.65 * g.at(0, 0.7, 'out'), () => {
      g.text('a-git', cx, 440 + rise, { size: 220, font: 'display', weight: 800, color: C.ink, align: 'center', spacing: -4 });
    });
    g.alpha(g.at(0.5, 0.8, 'out'), () => {
      g.text('version control, explained simply', cx, 528 + rise, { size: 46, color: C.muted, align: 'center' });
    });

    // What you already know: three familiar words.
    const words = [['commit', C.amber], ['branch', C.teal], ['push & pull', C.blue]];
    g.alpha(g.vis(1.8), () => {
      g.text('you already know', cx, 650, { size: 24, color: C.dim, align: 'center', spacing: 1 });
    });
    const widths = words.map(([w]) => g.measure(w, { size: 30, font: 'mono', weight: 500 }) + 40);
    const gap = 30;
    let x = cx - (widths.reduce((a, b) => a + b, 0) + gap * (words.length - 1)) / 2;
    words.forEach(([w, col], i) => {
      g.alpha(g.at(2.2 + i * 0.45, 0.5, 'out'), () => g.chip(w, x, 712, { color: col, size: 30 }));
      x += widths[i] + gap;
    });

    // What changes: where the work happens.
    g.alpha(g.vis(7.4), () => {
      g.text('what changes: where the work happens', cx, 820, { size: 30, weight: 600, color: C.teal, align: 'center' });
    });
  },
});

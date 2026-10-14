// Beginner 12: recap in plain language, then a closing card.
AGX.scene({
  id: 'recap',
  order: 12,
  title: 'Recap',
  duration: 20,
  captions: [
    { at: 0, text: "So that's a-git. The history lives on the server, commits go straight there, and the server lines up everyone's changes." },
    { at: 9.4, text: 'You fetch only what you need, and copies close to you keep it all fast. Thanks for watching.' },
  ],
  draw(g, t) {
    const C = g.C;
    const outro = 1 - g.at(16.6, 0.6);
    const endCard = g.vis(17.2);

    g.alpha(outro, () => {
      g.heading('What to remember', { alpha: g.vis(0) });
      const items = [
        ['server', C.teal, 'History lives on the server', 'ask for what you need instead of copying it all', 0.6],
        ['bolt', C.amber, 'Commit goes straight there', 'no separate push step', 3.0],
        ['person', C.violet, 'The server lines up changes', 'two people can land at once without waiting', 5.6],
        ['folder', C.blue, 'Take only the folders you need', 'add more later, one folder at a time', 9.6],
        ['laptop', C.mint, 'Copies close to you', 'saved versions never change, so copies stay correct', 12.0],
      ];
      items.forEach(([icon, col, title, sub, at], i) => {
        const y = 330 + i * 110;
        g.alpha(g.vis(at), () => {
          g.circle(170, y, 40, { fill: g.rgba(col, 0.14), stroke: g.rgba(col, 0.7), lineWidth: 2 });
          g.icon(icon, 170, y, 42, { color: col, stroke: 8 });
          g.text(title, 240, y - 4, { size: 36, font: 'display', weight: 700, color: C.ink });
          g.text(sub, 240, y + 34, { size: 23, color: C.muted });
        });
      });
    });

    g.alpha(endCard, () => {
      g.text('a-git', 960, 530, { size: 140, font: 'display', weight: 800, color: C.teal, align: 'center' });
      g.text('version control, explained simply', 960, 606, { size: 32, color: C.muted, align: 'center' });
    });
  },
});

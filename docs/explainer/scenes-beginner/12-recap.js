// Beginner 12: recap in plain language, then a closing card.
// Left column: how your work flows. Right column: everything around it.
AGX.scene({
  id: 'recap',
  order: 12,
  title: 'Recap',
  duration: 22,
  captions: [
    { at: 0, text: "So that's a-git. Your history lives on the server, commits go straight there, and landing puts your work on main." },
    { at: 9.0, text: 'You take only the folders you need, copies nearby keep it fast, and it works with Git.' },
    { at: 17.5, text: 'Thanks for watching!' },
  ],
  draw(g, t) {
    const C = g.C;
    const outro = 1 - g.at(17.0, 0.6);
    const endCard = g.vis(17.5);

    g.alpha(outro, () => {
      g.heading('What to remember', { alpha: g.vis(0) });
      const items = [
        // [icon, color, title, subtitle, appears at, column, row]
        ['server', C.teal, 'History lives on the server', 'ask for what you need instead of copying it all', 1.0, 0, 0],
        ['bolt', C.amber, 'Commit goes straight there', 'no separate push, but you need a connection', 3.5, 0, 1],
        ['check', C.teal, 'Land puts your work on main', 'the server puts your change on top for you', 5.6, 0, 2],
        ['folder', C.blue, 'Take only the folders you need', 'add more later; teams can protect their folders', 9.2, 1, 0],
        ['laptop', C.mint, 'Copies close to you', 'saved versions never change, so copies stay correct', 11.4, 1, 1],
        ['git', C.ink, 'Works with Git', 'bring your history in, copy it back out', 13.5, 1, 2],
      ];
      items.forEach(([icon, col, title, sub, at, c, r]) => {
        const x = c === 0 ? 170 : 1000;
        const y = 380 + r * 175;
        g.alpha(g.vis(at), () => {
          g.circle(x, y, 42, { fill: g.rgba(col, 0.14), stroke: g.rgba(col, 0.7), lineWidth: 2 });
          g.icon(icon, x, y, 44, { color: col, stroke: 8 });
          g.text(title, x + 72, y - 2, { size: 36, font: 'display', weight: 700, color: C.ink });
          g.text(sub, x + 72, y + 38, { size: 24, color: C.muted });
        });
      });
    });

    g.alpha(endCard, () => {
      g.text('a-git', 960, 530, { size: 140, font: 'display', weight: 800, color: C.teal, align: 'center' });
      g.text('version control, explained simply', 960, 606, { size: 32, color: C.muted, align: 'center' });
    });
  },
});

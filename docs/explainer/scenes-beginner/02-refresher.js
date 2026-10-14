// Beginner 2: a quick refresher on everyday Git.
AGX.scene({
  id: 'refresher',
  order: 2,
  title: 'A quick Git refresher',
  duration: 22,
  captions: [
    { at: 0, text: 'Quick refresher. In Git, you edit files, then save a commit: a snapshot of your project with a short message.' },
    { at: 8, text: 'Commits stay on your computer until you push them to a shared copy. Teammates pull to get your changes.' },
    { at: 15.5, text: 'And every person keeps a full copy of the whole project, including its entire history.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('How Git works today', { alpha: g.vis(0) });

    const you = { x: 300, y: 470 }, shared = { x: 960, y: 470 }, sam = { x: 1620, y: 470 };
    g.alpha(g.vis(0.2), () => BK.person(g, you.x, you.y, { name: 'You', color: C.blue }));
    g.alpha(g.vis(8.2), () => {
      g.node(shared.x, shared.y, { icon: 'server', label: 'shared copy', color: C.teal, w: 240, h: 190, labelFont: 'body', labelSize: 24 });
    });
    g.alpha(g.vis(10.8), () => BK.person(g, sam.x, sam.y, { name: 'Sam', color: C.violet }));

    // Commits pile up on your computer: each is a snapshot with a message.
    const msgs = ['add a page', 'fix a typo', 'add a picture'];
    msgs.forEach((m, i) => {
      const a = g.at(2.0 + i * 1.4, 0.5, 'outBack');
      if (a <= 0) return;
      const y = 650 + i * 64;
      g.alpha(Math.min(1, a), () => {
        g.circle(170, y, 14 * a, { fill: C.bg, stroke: C.amber, lineWidth: 4 });
        if (i > 0) g.line(170, y - 50, 170, y - 14, { color: C.amber, width: 3 });
        g.text(m, 200, y + 8, { size: 24, color: C.ink });
      });
    });
    g.alpha(g.vis(2.0), () => g.text('commits', 170, 610, { size: 20, color: C.amber, align: 'center', spacing: 1 }));

    // Push to the shared copy, then Sam pulls.
    const push = g.at(9.0, 1.3, 'inOut');
    g.arrow(you.x + 80, you.y - 20, shared.x - 130, shared.y - 20, { color: C.amber, width: 4, progress: push, headSize: 18 });
    g.alpha(g.vis(9.4), () => g.chip('push', (you.x + shared.x) / 2, you.y - 64, { color: C.amber, size: 26, align: 'center' }));
    const pull = g.at(11.6, 1.3, 'inOut');
    g.arrow(shared.x + 130, shared.y - 20, sam.x - 80, sam.y - 20, { color: C.violet, width: 4, progress: pull, headSize: 18 });
    g.alpha(g.vis(12.0), () => g.chip('pull', (shared.x + sam.x) / 2, sam.y - 64, { color: C.violet, size: 26, align: 'center' }));

    // Everyone holds a full copy of the history.
    const full = g.vis(15.8);
    g.alpha(full, () => {
      BK.stack(g, you.x, 820, { scale: 0.8, label: 'full history' });
      BK.stack(g, shared.x, 760, { scale: 0.8, label: 'full history' });
      BK.stack(g, sam.x, 820, { scale: 0.8, label: 'full history' });
    });
  },
});

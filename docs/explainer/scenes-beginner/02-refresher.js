// Beginner 2: a quick refresher on everyday Git.
AGX.scene({
  id: 'refresher',
  order: 2,
  title: 'A quick Git refresher',
  duration: 23,
  captions: [
    { at: 0, text: 'Quick refresher. In Git, you edit files, then save a commit: a snapshot of your project with a short message.' },
    { at: 8.2, text: 'Commits stay on your computer until you push them to the server. Teammates pull to get your changes.' },
    { at: 16.0, text: 'And every person keeps a full copy of the whole project, including its entire history.' },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('How Git works today', { alpha: g.vis(0) });

    const rowY = 450;
    const you = { x: 330, y: rowY }, server = { x: 960, y: rowY }, sam = { x: 1590, y: rowY };
    const arrowY = rowY - 20;
    const stackY = 760;
    const recap = 16.0; // caption 3: commits fold away, full histories appear

    // "Your computer" and "Sam's computer": where each person's commits live.
    const area = (x, w, label, col, start) => g.alpha(g.vis(start) * 0.9, () => {
      g.box(x, 318, w, 554, { stroke: g.rgba(col, 0.35), lineWidth: 2, radius: 22, dash: [8, 8] });
      g.text(label, x + 22, 356, { size: 22, color: C.muted });
    });
    area(140, 620, 'your computer', C.blue, 8.3);
    area(1390, 400, "Sam's computer", C.violet, 12.2);

    g.alpha(g.vis(0.2), () => BK.person(g, you.x, you.y, { name: 'You', color: C.blue }));
    g.alpha(g.vis(8.8), () => {
      g.node(server.x, server.y, {
        icon: 'server', label: 'server', sub: 'shared copy', color: C.teal, w: 250, h: 190,
        labelFont: 'body', labelSize: 28, subSize: 21,
      });
    });
    g.alpha(g.vis(12.2), () => BK.person(g, sam.x, sam.y, { name: 'Sam', color: C.violet }));

    // You edit a file, then save a commit: a snapshot with a short message.
    const fade = 1 - g.at(recap, 0.6, 'out');
    const file = { x: you.x, y: 636 };
    const list = { x: 548, y0: 650, step: 62 };
    const msgs = ['add a title', 'fix a typo', 'add a photo'];
    const edits = msgs.map((m, i) => 1.4 + i * 1.9);
    const commits = edits.map((e) => e + 1.0);
    g.alpha(g.vis(0.8) * fade, () => {
      const editing = edits.some((e, i) => t >= e && t < commits[i]);
      BK.file(g, file.x, file.y, 'home.html', { state: editing ? 'edited' : 'same', w: 220 });
      g.alpha(g.vis(2.0), () => g.text('commits', list.x, list.y0 - 44, { size: 22, color: C.amber, align: 'center', spacing: 1 }));
      msgs.forEach((m, i) => {
        const y = list.y0 + i * list.step;
        // The snapshot flies from the file to its place in the list.
        const fly = g.at(commits[i] - 0.5, 0.5, 'inOut');
        if (fly > 0 && fly < 1) {
          const fx = g.lerp(file.x + 110, list.x, fly), fy = g.lerp(file.y, y, fly);
          g.box(fx - 18, fy - 13, 36, 26, { fill: g.rgba(C.amber, 0.25), stroke: C.amber, radius: 6, lineWidth: 2 });
        }
        const a = g.at(commits[i], 0.5, 'outBack');
        if (a <= 0) return;
        g.alpha(Math.min(1, a), () => {
          if (i > 0) g.line(list.x, y - list.step + 14, list.x, y - 14, { color: C.amber, width: 3 });
          g.circle(list.x, y, 14 * a, { fill: C.bg, stroke: C.amber, lineWidth: 4 });
          g.text(m, list.x + 30, y + 8, { size: 24, color: C.ink });
        });
      });
    });

    // A small row of commits under the server or Sam once they arrive.
    const row = (cx, arrive) => {
      for (let i = 0; i < 3; i++) {
        const a = g.at(arrive[i], 0.35, 'outBack');
        if (a <= 0) continue;
        const x = cx - 44 + i * 44, y = 600;
        g.alpha(Math.min(1, a), () => {
          if (i > 0) g.line(x - 44 + 10, y, x - 10, y, { color: C.amber, width: 3 });
          g.circle(x, y, 10 * a, { fill: C.bg, stroke: C.amber, lineWidth: 3 });
        });
      }
    };
    // Commits travel along an arrow, one after another.
    const travel = (x1, x2, start) => {
      const done = [];
      for (let i = 0; i < 3; i++) {
        const s = start + i * 0.3;
        g.packet([[x1, arrowY], [x2, arrowY]], g.at(s, 0.9, 'inOut'), { color: C.amber, r: 9 });
        done.push(s + 0.9);
      }
      return done;
    };

    // Push your commits to the server.
    const pushX1 = you.x + 80, pushX2 = server.x - 135;
    g.arrow(pushX1, arrowY, pushX2, arrowY, { color: C.amber, width: 4, progress: g.at(9.6, 1.0, 'inOut'), headSize: 18 });
    g.alpha(g.vis(10.0), () => g.chip('push', (pushX1 + pushX2) / 2, rowY - 66, { color: C.amber, size: 26, align: 'center' }));
    const atServer = travel(pushX1, pushX2, 10.6);
    g.alpha(fade, () => row(server.x, atServer));

    // Sam pulls them from the server.
    const pullX1 = server.x + 135, pullX2 = sam.x - 80;
    g.arrow(pullX1, arrowY, pullX2, arrowY, { color: C.violet, width: 4, progress: g.at(12.8, 1.0, 'inOut'), headSize: 18 });
    g.alpha(g.vis(13.2), () => g.chip('pull', (pullX1 + pullX2) / 2, rowY - 66, { color: C.violet, size: 26, align: 'center' }));
    const atSam = travel(pullX1, pullX2, 13.8);
    g.alpha(fade, () => row(sam.x, atSam));

    // Everyone holds a full copy of the whole project and its history.
    [you, server, sam].forEach((p, i) => {
      const s = recap + 0.4 + i * 0.4;
      const u = g.at(s + 2.2, 0.9, 'linear');
      const bump = u > 0 && u < 1 ? Math.sin(Math.PI * u) : 0;
      g.alpha(g.vis(s), () => {
        g.transform({ scale: 1 + 0.06 * bump, cx: p.x, cy: stackY - 40 }, () => {
          BK.stack(g, p.x, stackY, { scale: 0.9, label: 'full history', labelSize: 22 });
        });
      });
    });
  },
});

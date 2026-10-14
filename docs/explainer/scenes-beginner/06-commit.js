// Beginner 6: committing goes straight to the server; a changeset is a list of
// edits; a new commit is saved on the server but is not on main until you land
// it (Commit never moves a ref: internal/workcopy/commit.go).
AGX.scene({
  id: 'commit',
  order: 6,
  title: 'Committing',
  duration: 26,
  captions: [
    { at: 0, text: 'When you commit in a-git, your change goes straight to the server. No separate push, but you do need a connection.' },
    { at: 9.2, text: 'Each commit, called a changeset, records only what you changed: which files were edited, added or removed.' },
    { at: 17.6, text: "It's saved on the server, but it isn't on main yet. It joins main for everyone when you land it." },
  ],
  draw(g, t) {
    const C = g.C;
    const BK = window.BK;
    g.heading('Commit goes straight to the server', { alpha: g.vis(0) });

    const Y = 440; // the row where your commit travels to the server

    // You, and the command you run.
    g.alpha(g.vis(0.3), () => BK.person(g, 250, Y, { name: 'You', color: C.blue }));
    g.alpha(g.vis(1.0), () => g.chip('agit commit', 250, 600, { color: C.blue, size: 26, align: 'center' }));

    // The server, which also holds main.
    const px = 1130, py = 290, pw = 660, ph = 590;
    const mainY = 750, dots = [1240, 1370, 1500], ghost = 1640;
    g.alpha(g.vis(0.6), () => {
      g.box(px, py, pw, ph, { fill: C.panel, stroke: g.rgba(C.teal, 0.6), radius: 18 });
      g.icon('server', px + 56, py + 52, 46, { color: C.teal, stroke: 7 });
      g.text('a-git server', px + 96, py + 62, { size: 28, weight: 600, color: C.ink });
      g.text('main', px + 50, mainY - 46, { size: 26, font: 'mono', weight: 600, color: C.teal });
      g.line(px + 50, mainY, px + pw - 40, mainY, { color: C.lineSoft, width: 4, dash: [2, 12] });
      g.line(px + 50, mainY, dots[2], mainY, { color: C.teal, width: 6 });
      dots.forEach((x) => g.circle(x, mainY, 16, { fill: C.bg, stroke: C.teal, lineWidth: 5 }));
    });

    // One step: the commit travels straight to the server.
    g.arrow(340, Y, 1300, Y, { color: C.teal, width: 4, progress: g.at(1.6, 1.6, 'inOut'), headSize: 18 });
    g.packet([[340, Y], [1300, Y]], g.at(1.6, 1.6, 'inOut'), { color: C.amber, r: 12 });

    // Your commit, now stored on the server (beside main, not on it).
    const tx = 1460, tw = 300, th = 76;
    const pop = g.at(3.1, 0.5, 'outBack');
    const glow = g.vis(9.4, 17.4) > 0.5;
    if (pop > 0) {
      g.transform({ scale: 0.6 + 0.4 * pop, cx: tx, cy: Y }, () => g.alpha(Math.min(1, pop * 1.5), () => {
        g.box(tx - tw / 2, Y - th / 2, tw, th, {
          fill: g.rgba(C.amber, 0.16), stroke: g.rgba(C.amber, 0.9), radius: 14, lineWidth: 2.5,
          shadow: glow ? g.rgba(C.amber, 0.5) : null,
        });
        g.icon('file', tx - tw / 2 + 36, Y, 32, { color: C.amber, stroke: 9 });
        g.text('your commit', tx - tw / 2 + 66, Y + 9, { size: 26, weight: 600, color: C.ink });
      }));
    }

    // The old two-step flow, crossed out, and the trade-off.
    g.alpha(g.vis(4.0, 17.2), () => {
      const x = 560, y = Y - 60;
      const label = 'commit, then push later';
      g.text(label, x, y, { size: 26, color: C.muted });
      const w = g.measure(label, { size: 26 });
      g.line(x - 6, y - 9, x + w + 6, y - 9, { color: C.coral, width: 3, progress: g.at(4.4, 0.5) });
      g.text('one step', x + w + 30, y, { size: 26, weight: 600, color: C.mint, alpha: g.at(5.0, 0.4) });
    });
    g.alpha(g.vis(6.2, 17.2), () => g.text('needs a connection to the server', 820, Y + 52, { size: 22, color: C.muted, align: 'center' }));

    // The changeset: a short list of edits, not a copy of every file.
    g.alpha(g.vis(9.4, 17.4), () => {
      const cx = 440, cy = 540;
      g.line(cx + 560, cy + 20, tx - tw / 2 + 10, Y + th / 2, { color: g.rgba(C.amber, 0.55), width: 2, dash: [6, 7] });
      g.card(cx, cy, 560, 330, { title: 'changeset', titleSize: 22, color: C.amber });
      const rows = [['edited', 'app.go', C.amber], ['added', 'helper.go', C.mint], ['removed', 'old.md', C.coral]];
      rows.forEach(([verb, name, col], i) => {
        g.alpha(g.at(10.4 + i * 1.0, 0.4), () => {
          BK.file(g, cx + 180, cy + 90 + i * 72, name, { state: verb, w: 280, h: 56 });
          g.text(verb, cx + 350, cy + 98 + i * 72, { size: 24, weight: 600, color: col });
        });
      });
      g.text('only what changed', cx + 30, cy + 306, { size: 22, color: C.muted, alpha: g.at(13.4, 0.5) });
    });

    // Saved on the server, but not on main yet.
    g.alpha(g.vis(17.9), () => g.chip('saved on the server', tx, 540, { color: C.mint, font: 'body', size: 24, align: 'center' }));
    g.alpha(g.vis(19.0), () => g.chip('not on main yet', tx, 604, { color: C.muted, textColor: C.ink, font: 'body', size: 24, align: 'center' }));
    // Landing (next) is what puts it on main for everyone.
    const land = g.at(21.6, 1.0, 'inOut');
    if (land > 0) {
      g.arrow(tx + tw / 2 - 16, Y + th / 2 + 4, ghost, mainY - 26, { color: C.teal, width: 3, dash: [8, 8], progress: land, bend: -30, headSize: 14 });
      g.circle(ghost, mainY, 16, { fill: C.bg, stroke: g.rgba(C.teal, 0.8), lineWidth: 3, dash: [5, 5], alpha: land });
      g.text('land', ghost + 34, 640, { size: 24, weight: 600, color: C.teal, alpha: g.at(23.2, 0.4) });
    }
    // Your teammates build on main, so that is where they will see it.
    g.alpha(g.vis(22.4), () => {
      BK.person(g, 560, 720, { name: 'Robin', color: C.blue, size: 0.75 });
      BK.person(g, 740, 720, { name: 'Sam', color: C.violet, size: 0.75 });
      g.text('everyone builds on main', 650, 856, { size: 22, color: C.muted, align: 'center' });
    });
    g.arrow(px + 40, mainY, 830, 724, { color: g.rgba(C.teal, 0.7), width: 3, dash: [8, 8], progress: g.at(22.6, 0.8), headSize: 14 });
  },
});

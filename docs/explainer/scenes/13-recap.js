// Scene 13: recap of the five ideas, how to try it, and a closing card.
AGX.scene({
  id: 'recap',
  order: 13,
  title: 'Recap',
  duration: 16,
  captions: [
    { at: 0, text: 'Always-online clients, a server that does the work, changesets + derived data, lock-free commits, tiered caches.' },
    { at: 8.0, text: "That's a-git. scripts/demo.sh builds it, starts all three tiers and drives the main workflows." },
  ],
  draw(g, t) {
    const C = g.C;
    const ctx = g.ctx;

    // An open padlock (shackle lifted, right leg free), same proportions as
    // the engine's 'lock' icon.
    const openLock = (x, y, s, color, stroke) => {
      ctx.save();
      ctx.translate(x, y);
      ctx.scale(s / 100, s / 100);
      ctx.fillStyle = color;
      ctx.strokeStyle = color;
      ctx.lineWidth = stroke;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      ctx.beginPath(); ctx.roundRect(-30, 4, 60, 44, 8); ctx.fill();
      ctx.beginPath();
      ctx.moveTo(-20, 4); ctx.lineTo(-20, -18);
      ctx.arc(0, -18, 20, Math.PI, 0);
      ctx.lineTo(20, -10);
      ctx.stroke();
      ctx.restore();
    };

    const ROW0 = 0.4, STEP = 1.3;   // rows land as the caption reaches them
    const outro = 1 - g.at(14.4, 0.6);
    const endCard = g.vis(14.7) * (1 - g.at(15.5, 0.5));

    g.alpha(outro, () => {
      g.heading('a-git in five ideas', { alpha: g.vis(0) });

      const ideas = [
        ['agent', C.blue, 'Clients are always online', 'no local history; every read and write is an API call'],
        ['server', C.teal, 'The server does the work', 'commits, merges, rebases and lands run there'],
        ['file', C.amber, 'Changesets + derived data', 'store only what changed; compute the rest from parents'],
        ['unlock', C.mint, 'Lock-free commits', 'idempotent inserts; refs move by per-ref compare-and-swap'],
        ['db', C.violet, 'Caches at every tier', 'immutable objects, fetched only when missing'],
      ];
      ideas.forEach(([icon, col, title, sub], i) => {
        const y = 330 + i * 108;
        const land = ROW0 + i * STEP;
        // Light up the row being read, then settle.
        const hi = g.at(land, 0.3, 'out') * (1 - g.at(land + 0.8, 0.6, 'in'));
        g.alpha(g.at(land, 0.5), () => {
          g.circle(160, y, 38, {
            fill: g.rgba(col, 0.12 + 0.2 * hi), stroke: g.rgba(col, 0.6 + 0.4 * hi),
            lineWidth: 2 + hi, glow: hi > 0.01 ? g.rgba(col, 0.5 * hi) : null,
          });
          if (icon === 'unlock') openLock(160, y - 3, 40, col, 8);
          else g.icon(icon, 160, y, 40, { color: col, stroke: 8 });
          g.text(title, 230, y - 6, { size: 32, font: 'display', weight: 700, color: C.ink });
          g.text(sub, 230, y + 30, { size: 21, color: C.muted });
        });
      });

      // Try it.
      g.alpha(g.vis(7.2), () => {
        const o = g.terminal(1060, 300, 750, 370, { title: 'try it' });
        g.code([
          [['$ ', C.teal], ['./scripts/demo.sh', C.ink]],
          [['==> build', C.muted]],
          [['==> start agit-server <- agit-cache <- agitd', C.muted]],
          [['==> sparse clone through the daemon', C.muted]],
          [['==> alice lands: the server rebases her stack', C.muted]],
          [['demo completed successfully', C.mint]],
        ], o.x, o.y, { size: 22, lineHeight: 50, progress: g.at(7.8, 3.6, 'linear') });
      });
      g.alpha(g.vis(11.6), () => {
        const w = g.text('docs/DESIGN.md', 1060, 740, { size: 24, font: 'mono', color: C.ink });
        g.text(' has the full design', 1060 + w, 740, { size: 24, color: C.muted });
        g.text('a-git', 1060, 812, { size: 54, font: 'display', weight: 800, color: C.teal });
      });
    });

    // Closing card.
    g.alpha(endCard, () => {
      g.text('a-git', 960, 540, { size: 120, font: 'display', weight: 800, color: C.teal, align: 'center' });
      g.text('a server-centric version control system', 960, 612, { size: 30, color: C.muted, align: 'center' });
    });
  },
});

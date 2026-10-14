// Scene 13: recap of the five ideas and how to try it.
AGX.scene({
  id: 'recap',
  order: 13,
  title: 'Recap',
  duration: 14,
  captions: [
    { at: 0, text: 'Always-online clients, a server that does the work, changesets plus derived data, lock-free writes, and caches at every tier.' },
    { at: 7, text: "That's a-git. Build it with go build, then run scripts/demo.sh to watch every piece work together." },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('a-git in five ideas', { alpha: g.vis(0) });

    const ideas = [
      ['agent', C.blue, 'Clients are always online', 'no local history; every read and write is an API call'],
      ['server', C.teal, 'The server does the work', 'commits, merges, rebases and lands run there'],
      ['file', C.amber, 'Changesets + derived data', 'store only what changed; compute the rest from parents'],
      ['lock', C.coral, 'Lock-free writes', 'idempotent inserts; refs move by compare-and-swap'],
      ['db', '#7FB8E8', 'Caches at every tier', 'immutable objects, fetched only when missing'],
    ];
    ideas.forEach(([icon, col, title, sub], i) => {
      const y = 330 + i * 108;
      g.alpha(g.stagger(i, 0.4, 0.5, 0.5), () => {
        g.circle(160, y, 38, { fill: g.rgba(col, 0.12), stroke: g.rgba(col, 0.6) });
        g.icon(icon, 160, y, 40, { color: col, stroke: 8 });
        g.text(title, 230, y - 6, { size: 32, font: 'display', weight: 700, color: C.ink });
        g.text(sub, 230, y + 30, { size: 21, color: C.muted });
      });
    });

    // Try it.
    g.alpha(g.vis(6.6), () => {
      const o = g.terminal(1060, 300, 750, 420, { title: 'try it' });
      g.code([
        [['$ ', C.teal], ['go build -o bin/ ./cmd/...', C.ink]],
        [['$ ', C.teal], ['./scripts/demo.sh', C.ink]],
        [['==> start agit-server <- agit-cache <- agitd', C.muted]],
        [['==> sparse clone through the daemon', C.muted]],
        [['==> alice lands: the server rebases her stack', C.muted]],
        [['demo completed successfully', C.mint]],
      ], o.x, o.y, { size: 22, lineHeight: 50, progress: g.at(7.0, 3.6, 'linear') });
    });
    g.alpha(g.vis(10.8), () => {
      g.text('docs/DESIGN.md has the full design', 1060, 790, { size: 24, color: C.muted });
      g.text('a-git', 1060, 860, { size: 54, font: 'display', weight: 800, color: C.teal });
    });
  },
});

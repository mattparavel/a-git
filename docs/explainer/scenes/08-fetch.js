// Scene 8: the client-driven fetch protocol through the daemon, sparse
// checkouts, and resuming a broken object stream.
//
// What the code does (internal/fetch, internal/workcopy/checkout.go,
// internal/tier/proxy.go, internal/client/client.go):
//   - `agit clone` walks trees level by level (root, src/, then app/ + lib/),
//     then asks for the blobs in one batch. The working copy keeps no object
//     store; requests go to agitd when its socket exists.
//   - agitd answers cached objects first and fetches only its misses upstream.
//   - Responses are id/len/bytes frames. If the stream breaks, GetObjects asks
//     again for just the objects that did not arrive.
AGX.scene({
  id: 'fetch',
  order: 8,
  title: 'Fetching only what is missing',
  duration: 19,
  captions: [
    { at: 0, text: 'The client walks the tree one level at a time. Its daemon, agitd, serves cache hits and asks upstream for the rest.' },
    { at: 7.8, text: 'A sparse checkout just never asks for paths outside its profile.' },
    { at: 12.2, text: "Objects are named by hash, so a broken transfer resumes by asking only for what didn't arrive." },
  ],
  draw(g, t) {
    const C = g.C;
    g.heading('Ask only for what you need', { alpha: g.vis(0) });
    g.alpha(g.vis(0.3), () => g.chip('agit clone demo --sparse src', 110, 312, { color: C.blue, size: 21 }));

    // ---- timeline
    const REQ = [1.0, 2.6, 4.2, 10.0, 14.5]; // object requests 1-5
    const SPARSE = 7.8; // out-of-profile paths fade (caption 2)
    const STREAM = 11.9; // response to request 4 (caption 3 starts at 12.2)

    // Response stream for request 4: agitd writes its cache hit (lib.go) first,
    // then streams the misses it fetched upstream. helper.go is cut off.
    const fx = 1080, fy = 750, fh = 44, gap = 6, groupGap = 18;
    const FW = { id: 46, len: 50 };
    const objs = [
      { name: 'lib.go', bytes: 96, at: 12.0, grow: 0.25, color: C.mint },
      { name: 'app.go', bytes: 110, at: 12.6, grow: 0.25, color: C.amber },
      { name: 'helper.go', bytes: 150, at: 13.2, grow: 0.8, cut: 0.5, color: C.amber },
    ];
    {
      let x = fx;
      for (const o of objs) {
        o.x = x;
        o.bx = x + FW.id + gap + FW.len + gap; // where the bytes frame starts
        o.w = o.bx + o.bytes - x;
        o.done = o.at + 0.3 + o.grow * (o.cut || 1);
        x += o.w + groupGap;
      }
    }
    const helper = objs[2];
    const BREAK = helper.done; // the stream breaks partway through helper.go's bytes
    const RESUME = 15.0; // resumed response (after request 5) starts
    const RESUME_DONE = RESUME + 0.3 + 0.5;

    // When each tree node turns amber (fetched) or mint (agitd cache hit).
    const arriveAt = {
      root: REQ[0] + 0.8,
      src: REQ[1] + 0.8,
      lib: REQ[2] + 0.35, // cache hit: agitd answers it before fetching app/ upstream
      app: REQ[2] + 0.8,
      libgo: objs[0].done,
      appgo: objs[1].done,
      helper: RESUME_DONE,
    };

    // ---- tree on the left
    const N = {
      root: { label: '/', x: 560, y: 380 },
      docs: { label: 'docs/', x: 260, y: 510, parent: 'root', out: true },
      src: { label: 'src/', x: 560, y: 510, parent: 'root' },
      tools: { label: 'tools/', x: 860, y: 510, parent: 'root', out: true },
      app: { label: 'app/', x: 440, y: 640, parent: 'src' },
      lib: { label: 'lib/', x: 690, y: 640, parent: 'src', cached: true },
      guide: { label: 'guide.md', x: 260, y: 640, parent: 'docs', out: true, file: true },
      build: { label: 'build.sh', x: 900, y: 640, parent: 'tools', out: true, file: true },
      appgo: { label: 'app.go', x: 340, y: 770, parent: 'app', file: true },
      helper: { label: 'helper.go', x: 540, y: 770, parent: 'app', file: true },
      libgo: { label: 'lib.go', x: 690, y: 770, parent: 'lib', file: true, cached: true },
    };
    const fade = g.at(SPARSE, 0.5);
    g.alpha(g.vis(0.4), () => {
      for (const n of Object.values(N)) {
        if (!n.parent) continue;
        const p = N[n.parent];
        g.line(p.x, p.y + 22, n.x, n.y - 22, { color: n.out ? C.lineSoft : C.line, width: 2, alpha: n.out ? 1 - 0.5 * fade : 1 });
      }
      for (const [id, n] of Object.entries(N)) {
        const f = n.out ? fade : 0;
        const at = arriveAt[id];
        const u = at === undefined ? 0 : g.at(at, 0.3, 'out');
        const pulse = at === undefined ? 0 : g.at(at, 0.45, 'pulse');
        const target = n.cached ? C.mint : C.amber;
        const col = g.mix(C.muted, target, u);
        const bw = Math.max(92, g.measure(n.label, { size: 20, font: 'mono' }) + 32);
        const x0 = n.x - bw / 2, y0 = n.y - 22, r = n.file ? 22 : 8;
        g.alpha(1 - 0.65 * f, () => g.transform({ scale: 1 + 0.06 * pulse, cx: n.x, cy: n.y }, () => {
          g.box(x0, y0, bw, 44, { fill: C.panel, radius: r });
          if (u > 0) g.box(x0, y0, bw, 44, { fill: g.rgba(target, 0.16 * u), radius: r });
          if (f < 1) g.box(x0, y0, bw, 44, { stroke: g.rgba(col, 0.85 * (1 - f)), radius: r });
          if (f > 0) g.box(x0, y0, bw, 44, { stroke: g.rgba(col, 0.85 * f), radius: r, dash: [6, 6] });
          g.text(n.label, n.x, n.y + 7, { size: 20, font: 'mono', color: C.ink, align: 'center' });
        }));
      }
    });

    // lib/ and lib.go come from agitd's cache.
    g.alpha(g.vis(arriveAt.lib), () => {
      g.line(738, 652, 788, 754, { color: g.rgba(C.mint, 0.6), width: 2, dash: [4, 5] });
      g.chip('agitd cache hit', 762, 770, { color: C.mint, size: 18 });
    });

    // Legend for the faded, dashed nodes, centred under the tree.
    g.alpha(g.vis(SPARSE + 0.3), () => {
      const label = 'outside the profile: never requested';
      const tw = g.measure(label, { size: 20 });
      const x0 = 580 - (28 + 12 + tw) / 2;
      g.box(x0, 847, 28, 18, { stroke: g.rgba(C.muted, 0.6), radius: 5, dash: [5, 4] });
      g.text(label, x0 + 40, 862, { size: 20, color: C.muted });
    });

    // ---- object request log on the right
    const lx = 1080, ly = 290;
    g.alpha(g.vis(0.5), () => g.card(lx, ly, 730, 378, { title: 'object requests', color: C.blue }));
    const mintNote = g.rgba(C.mint, 0.9);
    const reqs = [
      { at: REQ[0], text: 'want root tree', n: '1 object' },
      { at: REQ[1], text: 'want src/', n: '1 object', note: 'docs/, tools/ skipped', noteColor: C.muted },
      { at: REQ[2], text: 'want app/, lib/', n: '2 objects', note: 'lib/: agitd cache hit', noteAt: arriveAt.lib, noteColor: mintNote },
      { at: REQ[3], text: 'want lib.go, app.go, helper.go', n: '3 blobs', note: 'lib.go: agitd cache hit', noteAt: arriveAt.libgo, noteColor: mintNote },
      { at: REQ[4], text: 'retry helper.go', n: '1 blob', retry: true, note: 'after connection reset', noteColor: C.muted },
    ];
    let yy = ly + 84;
    reqs.forEach((r, i) => {
      const y = yy;
      g.alpha(g.at(r.at, 0.35, 'out'), () => {
        g.text(String(i + 1), lx + 30, y, { size: 21, font: 'mono', color: r.retry ? C.coral : C.amber });
        g.text(r.text, lx + 70, y, { size: 22, font: 'mono', color: C.ink });
        g.text(r.n, lx + 700, y, { size: 19, font: 'mono', color: C.muted, align: 'right' });
      });
      if (r.note) {
        g.alpha(g.at(r.noteAt === undefined ? r.at : r.noteAt, 0.35, 'out'), () => {
          g.text(r.note, lx + 70, y + 28, { size: 18, color: r.noteColor });
        });
      }
      yy += r.note ? 64 : 46;
    });

    // ---- the response stream: 32-byte id, 4-byte len, then the bytes.
    const frame = (kind, x, y, w, col) => {
      let fill, stroke, tc;
      if (kind === 'len') { fill = C.panel; stroke = g.rgba(C.muted, 0.6); tc = C.muted; }
      else if (kind === 'id') { fill = g.rgba(col, 0.08); stroke = g.rgba(col, 0.45); tc = g.rgba(col, 0.75); }
      else { fill = g.rgba(col, 0.18); stroke = g.rgba(col, 0.75); tc = col; }
      g.box(x, y, w, fh, { fill, stroke, radius: 6 });
      if (w >= 60 || kind !== 'bytes') {
        g.text(kind, x + w / 2, y + 29, { size: 18, font: 'mono', color: tc, align: 'center' });
      }
    };
    // Draws one object's id/len/bytes frames starting at `at`; `cut` stops the bytes partway.
    const objectFrames = (o, x, y, at, grow, cut) => {
      g.alpha(g.at(at, 0.15, 'out'), () => frame('id', x, y, FW.id, o.color));
      g.alpha(g.at(at + 0.15, 0.15, 'out'), () => frame('len', x + FW.id + gap, y, FW.len, o.color));
      const p = Math.min(g.at(at + 0.3, grow, 'linear'), cut || 1);
      if (p > 0) frame('bytes', x + FW.id + gap + FW.len + gap, y, Math.max(6, o.bytes * p), o.color);
    };

    g.alpha(g.vis(STREAM), () => {
      g.text('response stream', fx, fy - 20, { size: 19, color: C.muted, spacing: 1 });
      const lost = 1 - 0.75 * g.at(BREAK, 0.25);
      for (const o of objs) {
        g.alpha(o === helper ? lost : 1, () => objectFrames(o, o.x, fy, o.at, o.grow, o.cut));
        g.alpha(g.at(o.at, 0.3, 'out'), () => {
          g.text(o.name, o.x + o.w / 2, fy + fh + 20, { size: 17, font: 'mono', color: g.rgba(o.color, 0.9), align: 'center' });
        });
      }

      // The break: the rest of helper.go's bytes never arrives. Outline that
      // gap and put the cross inside it, clear of every frame.
      const cutX = helper.bx + helper.bytes * helper.cut;
      const restW = helper.bytes * (1 - helper.cut) - 4;
      g.alpha(g.at(BREAK, 0.25, 'out'), () => {
        g.box(cutX + 4, fy, restW, fh, { stroke: g.rgba(C.coral, 0.5), radius: 6, dash: [5, 5] });
        g.icon('cross', cutX + 4 + restW / 2, fy + fh / 2, 34, { color: C.coral, stroke: 5 });
      });

      // Status line under the stream, left-aligned with it.
      const sy = fy + 86;
      g.alpha(g.vis(BREAK, RESUME - 0.1, 0.25), () => {
        g.text('connection reset', fx, sy + 29, { size: 19, color: C.coral });
      });
      g.alpha(g.vis(RESUME, null, 0.3), () => {
        g.text('resumed: only helper.go', fx, sy + 29, { size: 19, color: C.mint });
      });
      // The resumed response carries helper.go alone, under its lost frames.
      objectFrames(helper, helper.x, sy, RESUME, 0.5);
    });
  },
});

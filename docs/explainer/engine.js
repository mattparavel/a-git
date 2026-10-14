/*
 * a-git explainer: a tiny deterministic animation engine.
 *
 * Every frame is a pure function of time: renderAt(t) draws the frame at t
 * seconds onto a fixed 1920x1080 canvas. The browser player calls it from
 * requestAnimationFrame; the offline renderer (scripts/explainer/render.mjs)
 * calls it once per video frame and encodes the result, so the MP4 and the
 * page show exactly the same pictures.
 *
 * Scenes live in scenes/*.js and register themselves with AGX.scene({...}).
 * A scene's draw(g, t, d) receives a graphics helper g, the scene-local time
 * t (seconds) and the scene duration d. Never use Math.random or Date in a
 * scene; use g.rand(seed) and g.hash(seed) instead.
 */
(function () {
  'use strict';

  const W = 1920, H = 1080;

  // Palette for the video itself (a single deliberate dark look).
  const C = {
    bg: '#0C181B',
    bg2: '#10232A',
    panel: '#13292F',
    panel2: '#183238',
    line: '#2A4950',
    lineSoft: '#1F3A40',
    ink: '#E8F1EF',
    muted: '#93ACA9',
    dim: '#5E7A77',
    teal: '#45C8B8',   // the server, primary accent
    amber: '#F4BA4E',  // content-addressed objects, hashes
    blue: '#8EA8FF',   // clients, agents, working copies
    coral: '#FF7C6E',  // deletions, conflicts, failures
    mint: '#8EDC8F',   // additions, success, approvals
    violet: '#C6A8FF', // derived data
  };

  const F = {
    display: '"Bricolage Grotesque", "Inter", system-ui, sans-serif',
    body: '"IBM Plex Sans", "Inter", system-ui, sans-serif',
    mono: '"IBM Plex Mono", "DejaVu Sans Mono", monospace',
  };

  // Content must stay inside this box; captions live below it.
  const SAFE = { x: 110, y: 150, w: 1700, h: 740 };
  const CAPTION_Y = 958;

  // ---------------------------------------------------------------- easing
  const ease = {
    linear: (u) => u,
    in: (u) => u * u * u,
    out: (u) => 1 - Math.pow(1 - u, 3),
    inOut: (u) => (u < 0.5 ? 4 * u * u * u : 1 - Math.pow(-2 * u + 2, 3) / 2),
    outBack: (u) => {
      const c1 = 1.70158, c3 = c1 + 1;
      return 1 + c3 * Math.pow(u - 1, 3) + c1 * Math.pow(u - 1, 2);
    },
    pulse: (u) => Math.sin(Math.PI * u),
  };

  const clamp01 = (u) => (u < 0 ? 0 : u > 1 ? 1 : u);
  const lerp = (a, b, u) => a + (b - a) * u;

  function hexToRgb(hex) {
    const h = hex.replace('#', '');
    const n = parseInt(h.length === 3 ? h.split('').map((c) => c + c).join('') : h, 16);
    return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
  }

  function rgba(hex, a) {
    const [r, g, b] = hexToRgb(hex);
    return `rgba(${r},${g},${b},${a})`;
  }

  function mix(hexA, hexB, u) {
    const a = hexToRgb(hexA), b = hexToRgb(hexB);
    const c = a.map((v, i) => Math.round(lerp(v, b[i], clamp01(u))));
    return '#' + c.map((v) => v.toString(16).padStart(2, '0')).join('');
  }

  // FNV-1a based deterministic helpers.
  function fnv(str) {
    let h = 0x811c9dc5;
    for (let i = 0; i < str.length; i++) {
      h ^= str.charCodeAt(i);
      h = Math.imul(h, 0x01000193) >>> 0;
    }
    return h >>> 0;
  }

  function hash(seed, n) {
    n = n || 12;
    let out = '';
    let h = fnv(String(seed));
    while (out.length < n) {
      h = Math.imul(h ^ (h >>> 15), 0x2c1b3c6d) >>> 0;
      h = Math.imul(h ^ (h >>> 12), 0x297a2d39) >>> 0;
      out += (h >>> 0).toString(16).padStart(8, '0');
    }
    return out.slice(0, n);
  }

  function rand(seed) {
    return fnv(String(seed) + '#') / 4294967296;
  }

  // ---------------------------------------------------------- graphics
  class Graphics {
    constructor(ctx) {
      this.ctx = ctx;
      this.t = 0;
      this.d = 1;
      this.C = C;
      this.F = F;
      this.W = W;
      this.H = H;
      this.SAFE = SAFE;
      this.ease = ease;
    }

    // Progress (0..1) of an animation that starts at `start` seconds and
    // lasts `dur` seconds, in scene-local time.
    at(start, dur, e) {
      const fn = typeof e === 'function' ? e : ease[e] || ease.inOut;
      if (dur <= 0) return this.t >= start ? 1 : 0;
      const u = (this.t - start) / dur;
      if (u <= 0) return 0;
      if (u >= 1) return 1;
      return fn(u);
    }

    // Fade in at `start`, optionally fade out at `end`.
    vis(start, end, fade) {
      fade = fade || 0.4;
      const a = this.at(start, fade, 'out');
      if (end === undefined || end === null) return a;
      return a * (1 - this.at(end, fade, 'in'));
    }

    // Staggered progress for the i-th of several items.
    stagger(i, start, step, dur, e) {
      return this.at(start + i * step, dur, e);
    }

    clamp01(u) { return clamp01(u); }
    lerp(a, b, u) { return lerp(a, b, u); }
    rgba(hex, a) { return rgba(hex, a); }
    mix(a, b, u) { return mix(a, b, u); }
    hash(seed, n) { return hash(seed, n); }
    rand(seed) { return rand(seed); }

    // Run fn with an extra alpha multiplier (skips drawing when invisible).
    alpha(a, fn) {
      if (a <= 0.001) return;
      const ctx = this.ctx;
      ctx.save();
      ctx.globalAlpha *= Math.min(1, a);
      fn();
      ctx.restore();
    }

    // Run fn translated/scaled around (cx, cy).
    transform(opts, fn) {
      const ctx = this.ctx;
      ctx.save();
      if (opts.x || opts.y) ctx.translate(opts.x || 0, opts.y || 0);
      if (opts.scale !== undefined && opts.scale !== 1) {
        const cx = opts.cx || 0, cy = opts.cy || 0;
        ctx.translate(cx, cy);
        ctx.scale(opts.scale, opts.scale);
        ctx.translate(-cx, -cy);
      }
      fn();
      ctx.restore();
    }

    font(o) {
      o = o || {};
      const fam = F[o.font || 'body'] || o.font;
      return `${o.italic ? 'italic ' : ''}${o.weight || 400} ${o.size || 28}px ${fam}`;
    }

    measure(str, o) {
      const ctx = this.ctx;
      ctx.save();
      ctx.font = this.font(o);
      if (o && o.spacing) ctx.letterSpacing = o.spacing + 'px';
      const w = ctx.measureText(str).width;
      ctx.restore();
      return w;
    }

    // Draw a single line of text. Returns its width.
    text(str, x, y, o) {
      o = o || {};
      const ctx = this.ctx;
      ctx.save();
      ctx.font = this.font(o);
      ctx.fillStyle = o.color || C.ink;
      ctx.textAlign = o.align || 'left';
      ctx.textBaseline = o.baseline || 'alphabetic';
      if (o.spacing) ctx.letterSpacing = o.spacing + 'px';
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      if (o.maxWidth) ctx.fillText(str, x, y, o.maxWidth);
      else ctx.fillText(str, x, y);
      const w = ctx.measureText(str).width;
      ctx.restore();
      return w;
    }

    // Break text into lines no wider than maxWidth.
    wrap(str, maxWidth, o) {
      const words = String(str).split(/\s+/);
      const lines = [];
      let cur = '';
      for (const w of words) {
        const cand = cur ? cur + ' ' + w : w;
        if (cur && this.measure(cand, o) > maxWidth) {
          lines.push(cur);
          cur = w;
        } else {
          cur = cand;
        }
      }
      if (cur) lines.push(cur);
      return lines;
    }

    // Draw wrapped text; returns the number of lines.
    para(str, x, y, maxWidth, o) {
      o = o || {};
      const lh = o.lineHeight || Math.round((o.size || 28) * 1.35);
      const lines = this.wrap(str, maxWidth, o);
      lines.forEach((l, i) => this.text(l, x, y + i * lh, o));
      return lines.length;
    }

    // Reveal the first fraction u of a string.
    typed(str, u) {
      const n = Math.floor(clamp01(u) * str.length + 1e-6);
      return str.slice(0, n);
    }

    box(x, y, w, h, o) {
      o = o || {};
      const ctx = this.ctx;
      ctx.save();
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      ctx.beginPath();
      ctx.roundRect(x, y, w, h, o.radius === undefined ? 14 : o.radius);
      if (o.shadow) {
        ctx.shadowColor = o.shadow;
        ctx.shadowBlur = o.shadowBlur || 30;
      }
      if (o.fill) {
        ctx.fillStyle = o.fill;
        ctx.fill();
      }
      ctx.shadowBlur = 0;
      if (o.stroke) {
        ctx.lineWidth = o.lineWidth || 2;
        ctx.strokeStyle = o.stroke;
        if (o.dash) ctx.setLineDash(o.dash);
        ctx.stroke();
      }
      ctx.restore();
    }

    // A labelled panel: rounded box with a small uppercase header.
    card(x, y, w, h, o) {
      o = o || {};
      const color = o.color || C.teal;
      this.box(x, y, w, h, {
        fill: o.fill || C.panel,
        stroke: o.stroke || rgba(color, 0.55),
        lineWidth: o.lineWidth || 2,
        radius: o.radius === undefined ? 16 : o.radius,
        dash: o.dash,
        shadow: o.glow ? rgba(color, 0.35) : null,
      });
      if (o.title) {
        this.text(o.title.toUpperCase(), x + 22, y + 36, {
          size: o.titleSize || 18, weight: 600, color, spacing: 2, font: 'body',
        });
      }
    }

    circle(x, y, r, o) {
      o = o || {};
      const ctx = this.ctx;
      ctx.save();
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      ctx.beginPath();
      ctx.arc(x, y, r, 0, Math.PI * 2);
      if (o.glow) {
        ctx.shadowColor = o.glow;
        ctx.shadowBlur = o.glowBlur || 24;
      }
      if (o.fill) {
        ctx.fillStyle = o.fill;
        ctx.fill();
      }
      ctx.shadowBlur = 0;
      if (o.stroke) {
        ctx.lineWidth = o.lineWidth || 2;
        ctx.strokeStyle = o.stroke;
        if (o.dash) ctx.setLineDash(o.dash);
        ctx.stroke();
      }
      ctx.restore();
    }

    // Pill-shaped label, e.g. a hash or a ref name. Returns its width.
    chip(label, x, y, o) {
      o = o || {};
      const size = o.size || 22;
      const font = o.font || 'mono';
      const padX = o.padX || Math.round(size * 0.6);
      const h = o.h || Math.round(size * 1.6);
      const w = this.measure(label, { size, font, weight: o.weight || 500 }) + padX * 2;
      const color = o.color || C.amber;
      let left = x;
      if (o.align === 'center') left = x - w / 2;
      if (o.align === 'right') left = x - w;
      this.box(left, y - h / 2, w, h, {
        fill: o.fill || rgba(color, 0.14),
        stroke: o.stroke === false ? null : o.stroke || rgba(color, 0.7),
        radius: h / 2,
        lineWidth: 1.5,
        alpha: o.alpha,
      });
      this.text(label, left + padX, y + size * 0.36, {
        size, font, weight: o.weight || 500, color: o.textColor || color, alpha: o.alpha,
      });
      return w;
    }

    line(x1, y1, x2, y2, o) {
      o = o || {};
      const u = o.progress === undefined ? 1 : clamp01(o.progress);
      if (u <= 0) return;
      const ctx = this.ctx;
      ctx.save();
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      ctx.strokeStyle = o.color || C.line;
      ctx.lineWidth = o.width || 2;
      ctx.lineCap = 'round';
      if (o.dash) ctx.setLineDash(o.dash);
      ctx.beginPath();
      ctx.moveTo(x1, y1);
      ctx.lineTo(lerp(x1, x2, u), lerp(y1, y2, u));
      ctx.stroke();
      ctx.restore();
    }

    // Points along a quadratic curve from a to b bending by `bend` pixels.
    curvePoints(x1, y1, x2, y2, bend, n) {
      n = n || 40;
      const mx = (x1 + x2) / 2, my = (y1 + y2) / 2;
      const dx = x2 - x1, dy = y2 - y1;
      const len = Math.hypot(dx, dy) || 1;
      const cx = mx - (dy / len) * (bend || 0);
      const cy = my + (dx / len) * (bend || 0);
      const pts = [];
      for (let i = 0; i <= n; i++) {
        const u = i / n;
        pts.push([
          (1 - u) * (1 - u) * x1 + 2 * (1 - u) * u * cx + u * u * x2,
          (1 - u) * (1 - u) * y1 + 2 * (1 - u) * u * cy + u * u * y2,
        ]);
      }
      return pts;
    }

    // Portion u (0..1) of a polyline, by length.
    partial(pts, u) {
      u = clamp01(u);
      if (u >= 1) return pts.slice();
      let total = 0;
      const seg = [];
      for (let i = 1; i < pts.length; i++) {
        const l = Math.hypot(pts[i][0] - pts[i - 1][0], pts[i][1] - pts[i - 1][1]);
        seg.push(l);
        total += l;
      }
      let want = total * u;
      const out = [pts[0]];
      for (let i = 1; i < pts.length; i++) {
        if (want >= seg[i - 1]) {
          out.push(pts[i]);
          want -= seg[i - 1];
        } else {
          const f = seg[i - 1] ? want / seg[i - 1] : 0;
          out.push([lerp(pts[i - 1][0], pts[i][0], f), lerp(pts[i - 1][1], pts[i][1], f)]);
          break;
        }
      }
      return out;
    }

    // Point at fraction u along a polyline.
    pointAt(pts, u) {
      const p = this.partial(pts, u);
      return p[p.length - 1];
    }

    path(pts, o) {
      o = o || {};
      if (o.progress !== undefined && o.progress <= 1e-6) return;
      const shown = o.progress === undefined ? pts : this.partial(pts, o.progress);
      if (shown.length < 2) return;
      const ctx = this.ctx;
      ctx.save();
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      ctx.strokeStyle = o.color || C.line;
      ctx.lineWidth = o.width || 2;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      if (o.dash) ctx.setLineDash(o.dash);
      ctx.beginPath();
      ctx.moveTo(shown[0][0], shown[0][1]);
      for (let i = 1; i < shown.length; i++) ctx.lineTo(shown[i][0], shown[i][1]);
      ctx.stroke();
      ctx.restore();
      if (o.head && (o.progress === undefined || o.progress > 0.02)) {
        const a = shown[shown.length - 2], b = shown[shown.length - 1];
        this.arrowHead(a[0], a[1], b[0], b[1], o);
      }
    }

    arrowHead(x1, y1, x2, y2, o) {
      o = o || {};
      const size = o.headSize || 14;
      const ang = Math.atan2(y2 - y1, x2 - x1);
      const ctx = this.ctx;
      ctx.save();
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      ctx.fillStyle = o.color || C.line;
      ctx.beginPath();
      ctx.moveTo(x2, y2);
      ctx.lineTo(x2 - size * Math.cos(ang - 0.42), y2 - size * Math.sin(ang - 0.42));
      ctx.lineTo(x2 - size * Math.cos(ang + 0.42), y2 - size * Math.sin(ang + 0.42));
      ctx.closePath();
      ctx.fill();
      ctx.restore();
    }

    // Arrow from (x1,y1) to (x2,y2); `bend` curves it; `progress` draws part.
    arrow(x1, y1, x2, y2, o) {
      o = o || {};
      const pts = o.bend ? this.curvePoints(x1, y1, x2, y2, o.bend) : [[x1, y1], [x2, y2]];
      this.path(pts, Object.assign({ head: true }, o));
      return pts;
    }

    // A glowing dot travelling along pts at fraction u, with optional label.
    packet(pts, u, o) {
      o = o || {};
      if (u <= 0 || u >= 1) return;
      const [x, y] = this.pointAt(pts, u);
      const color = o.color || C.amber;
      this.circle(x, y, o.r || 9, { fill: color, glow: rgba(color, 0.9), glowBlur: 22 });
      if (o.label) {
        this.chip(o.label, x + (o.dx || 16), y + (o.dy || -22), { size: o.size || 18, color });
      }
    }

    // Code or terminal lines. Each line is a string or an array of
    // [text, color] segments. `progress` reveals characters in order.
    code(lines, x, y, o) {
      o = o || {};
      const size = o.size || 24;
      const lh = o.lineHeight || Math.round(size * 1.55);
      let budget = o.progress === undefined ? Infinity : null;
      if (budget === null) {
        const total = lines.reduce((n, l) => n + (typeof l === 'string' ? l.length : l.reduce((m, s) => m + s[0].length, 0)), 0);
        budget = Math.floor(clamp01(o.progress) * total + 1e-6);
      }
      lines.forEach((l, i) => {
        const segs = typeof l === 'string' ? [[l, o.color || C.ink]] : l;
        let cx = x;
        for (const [txt, col] of segs) {
          if (budget <= 0) return;
          const shown = txt.slice(0, budget);
          budget -= shown.length;
          cx += this.text(shown, cx, y + i * lh, { size, font: 'mono', color: col || C.ink, weight: o.weight || 400 });
        }
      });
    }

    // A terminal window with a title bar. Returns the content origin.
    terminal(x, y, w, h, o) {
      o = o || {};
      this.box(x, y, w, h, { fill: '#08120F', stroke: C.line, radius: 14 });
      this.box(x, y, w, 44, { fill: C.panel2, radius: [14, 14, 0, 0] });
      [C.coral, C.amber, C.mint].forEach((c, i) => this.circle(x + 26 + i * 24, y + 22, 7, { fill: rgba(c, 0.85) }));
      if (o.title) this.text(o.title, x + w / 2, y + 29, { size: 18, color: C.muted, align: 'center', font: 'mono' });
      return { x: x + 26, y: y + 86 };
    }

    // Simple original icons drawn from primitives.
    icon(name, x, y, s, o) {
      o = o || {};
      const color = o.color || C.ink;
      const ctx = this.ctx;
      ctx.save();
      if (o.alpha !== undefined) ctx.globalAlpha *= o.alpha;
      ctx.translate(x, y);
      ctx.scale(s / 100, s / 100);
      ctx.strokeStyle = color;
      ctx.fillStyle = color;
      ctx.lineWidth = o.stroke || 7;
      ctx.lineCap = 'round';
      ctx.lineJoin = 'round';
      const rr = (a, b, w, h, r) => { ctx.beginPath(); ctx.roundRect(a, b, w, h, r); };
      switch (name) {
        case 'server':
          for (let i = 0; i < 3; i++) {
            rr(-40, -44 + i * 30, 80, 24, 6); ctx.stroke();
            ctx.beginPath(); ctx.arc(-24, -32 + i * 30, 4, 0, 7); ctx.fill();
          }
          break;
        case 'agent':
          rr(-36, -30, 72, 60, 16); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(0, -30); ctx.lineTo(0, -46); ctx.stroke();
          ctx.beginPath(); ctx.arc(0, -50, 6, 0, 7); ctx.fill();
          ctx.beginPath(); ctx.arc(-13, -4, 6, 0, 7); ctx.arc(13, -4, 6, 0, 7); ctx.fill();
          ctx.beginPath(); ctx.moveTo(-12, 14); ctx.lineTo(12, 14); ctx.stroke();
          break;
        case 'laptop':
          rr(-36, -30, 72, 48, 6); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(-48, 30); ctx.lineTo(48, 30); ctx.stroke();
          break;
        case 'ci':
          ctx.beginPath(); ctx.arc(0, 0, 34, 0.3, Math.PI * 1.8); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(30, -30); ctx.lineTo(34, -8); ctx.lineTo(12, -12); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(-10, -10); ctx.lineTo(12, 0); ctx.lineTo(-10, 10); ctx.closePath(); ctx.fill();
          break;
        case 'db':
          ctx.beginPath(); ctx.ellipse(0, -30, 36, 12, 0, 0, 7); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(-36, -30); ctx.lineTo(-36, 30); ctx.ellipse(0, 30, 36, 12, 0, Math.PI, 0, true); ctx.lineTo(36, -30); ctx.stroke();
          ctx.beginPath(); ctx.ellipse(0, 0, 36, 12, 0, 0, Math.PI); ctx.stroke();
          break;
        case 'lock':
          rr(-30, -6, 60, 46, 8); ctx.fill();
          ctx.beginPath(); ctx.arc(0, -10, 20, Math.PI, 0); ctx.lineTo(20, -6); ctx.moveTo(-20, -10); ctx.lineTo(-20, -6); ctx.stroke();
          break;
        case 'check':
          ctx.lineWidth = (o.stroke || 7) * 1.4;
          ctx.beginPath(); ctx.moveTo(-30, 2); ctx.lineTo(-8, 24); ctx.lineTo(32, -22); ctx.stroke();
          break;
        case 'cross':
          ctx.lineWidth = (o.stroke || 7) * 1.4;
          ctx.beginPath(); ctx.moveTo(-24, -24); ctx.lineTo(24, 24); ctx.moveTo(24, -24); ctx.lineTo(-24, 24); ctx.stroke();
          break;
        case 'file':
          ctx.beginPath(); ctx.moveTo(-28, -40); ctx.lineTo(12, -40); ctx.lineTo(30, -22); ctx.lineTo(30, 40); ctx.lineTo(-28, 40); ctx.closePath(); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(12, -40); ctx.lineTo(12, -22); ctx.lineTo(30, -22); ctx.stroke();
          break;
        case 'folder':
          ctx.beginPath(); ctx.moveTo(-40, -26); ctx.lineTo(-12, -26); ctx.lineTo(-4, -16); ctx.lineTo(40, -16); ctx.lineTo(40, 30); ctx.lineTo(-40, 30); ctx.closePath(); ctx.stroke();
          break;
        case 'bolt':
          ctx.beginPath(); ctx.moveTo(8, -44); ctx.lineTo(-22, 6); ctx.lineTo(0, 6); ctx.lineTo(-8, 44); ctx.lineTo(24, -8); ctx.lineTo(2, -8); ctx.closePath(); ctx.fill();
          break;
        case 'eye':
          ctx.beginPath(); ctx.moveTo(-42, 0); ctx.quadraticCurveTo(0, -40, 42, 0); ctx.quadraticCurveTo(0, 40, -42, 0); ctx.stroke();
          ctx.beginPath(); ctx.arc(0, 0, 12, 0, 7); ctx.fill();
          break;
        case 'person':
          // A simple figure: round head and rounded shoulders.
          ctx.beginPath(); ctx.arc(0, -22, 20, 0, 7); ctx.fill();
          ctx.beginPath(); ctx.moveTo(-36, 44); ctx.quadraticCurveTo(-36, 8, 0, 8); ctx.quadraticCurveTo(36, 8, 36, 44); ctx.closePath(); ctx.fill();
          break;
        case 'book':
          rr(-34, -40, 68, 80, 6); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(-18, -40); ctx.lineTo(-18, 40); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(-4, -18); ctx.lineTo(22, -18); ctx.moveTo(-4, 0); ctx.lineTo(22, 0); ctx.stroke();
          break;
        case 'git':
          ctx.beginPath(); ctx.moveTo(-20, -34); ctx.lineTo(-20, 34); ctx.stroke();
          ctx.beginPath(); ctx.moveTo(-20, 0); ctx.quadraticCurveTo(22, 0, 22, -26); ctx.stroke();
          [[-20, -34], [-20, 34], [22, -30]].forEach(([a, b]) => { ctx.beginPath(); ctx.arc(a, b, 10, 0, 7); ctx.fill(); });
          break;
        default:
          ctx.beginPath(); ctx.arc(0, 0, 30, 0, 7); ctx.stroke();
      }
      ctx.restore();
    }

    // A labelled node with an icon above its name.
    node(x, y, o) {
      o = o || {};
      const w = o.w || 200, h = o.h || 150;
      const color = o.color || C.teal;
      this.box(x - w / 2, y - h / 2, w, h, {
        fill: o.fill || C.panel, stroke: rgba(color, o.active ? 0.95 : 0.55), lineWidth: o.active ? 3 : 2,
        radius: 18, shadow: o.active ? rgba(color, 0.45) : null,
      });
      if (o.icon) this.icon(o.icon, x, y - h * 0.14, o.iconSize || 56, { color, stroke: 7 });
      if (o.label) this.text(o.label, x, y + h / 2 - (o.sub ? 44 : 22), { size: o.labelSize || 24, weight: 600, align: 'center', font: o.labelFont || 'mono', color: C.ink });
      if (o.sub) this.text(o.sub, x, y + h / 2 - 18, { size: o.subSize || 17, align: 'center', color: C.muted });
    }

    // Big section heading used at the top of scenes.
    heading(str, o) {
      o = o || {};
      const a = o.alpha === undefined ? 1 : o.alpha;
      this.text(str, o.x || SAFE.x, o.y || 232, { size: o.size || 64, font: 'display', weight: 700, color: C.ink, alpha: a });
    }
  }

  // ------------------------------------------------------------ engine
  const scenes = [];
  let background = null;

  function scene(def) {
    if (!def || !def.id || !def.duration || typeof def.draw !== 'function') {
      throw new Error('AGX.scene needs id, duration and draw');
    }
    scenes.push(def);
    scenes.sort((a, b) => (a.order || 0) - (b.order || 0));
  }

  function chapters() {
    let start = 0;
    return scenes.map((s, i) => {
      const c = { index: i, id: s.id, title: s.title, start, duration: s.duration, captions: s.captions || [] };
      start += s.duration;
      return c;
    });
  }

  function duration() {
    return scenes.reduce((n, s) => n + s.duration, 0);
  }

  function locate(t) {
    let start = 0;
    for (let i = 0; i < scenes.length; i++) {
      const s = scenes[i];
      if (t < start + s.duration || i === scenes.length - 1) {
        return { index: i, scene: s, local: Math.max(0, Math.min(s.duration, t - start)), start };
      }
      start += s.duration;
    }
    return null;
  }

  function captionAt(s, local) {
    const caps = s.captions || [];
    let cur = null, next = s.duration;
    for (let i = 0; i < caps.length; i++) {
      if (local >= caps[i].at) {
        cur = caps[i];
        next = i + 1 < caps.length ? caps[i + 1].at : s.duration;
      }
    }
    if (!cur) return null;
    return { text: cur.text, start: cur.at, end: next };
  }

  function drawBackground(ctx) {
    if (!background) {
      background = document.createElement('canvas');
      background.width = W;
      background.height = H;
      const b = background.getContext('2d');
      const grad = b.createRadialGradient(W * 0.5, H * 0.42, 100, W * 0.5, H * 0.5, W * 0.75);
      grad.addColorStop(0, C.bg2);
      grad.addColorStop(1, C.bg);
      b.fillStyle = grad;
      b.fillRect(0, 0, W, H);
      // Faint dot grid, like squared engineering paper.
      b.fillStyle = 'rgba(140,190,185,0.075)';
      for (let y = 30; y < H; y += 40) {
        for (let x = 30; x < W; x += 40) {
          b.beginPath();
          b.arc(x, y, 1.6, 0, Math.PI * 2);
          b.fill();
        }
      }
    }
    ctx.drawImage(background, 0, 0);
  }

  function drawChrome(g, loc, total) {
    const { scene: s, index } = loc;
    // Top-left: wordmark and chapter.
    g.text('a-git', 110, 92, { size: 30, font: 'display', weight: 800, color: C.teal });
    const w = g.measure('a-git', { size: 30, font: 'display', weight: 800 });
    g.text(String(index + 1).padStart(2, '0'), 110 + w + 30, 91, { size: 22, font: 'mono', weight: 600, color: C.amber });
    g.text(s.title || '', 110 + w + 72, 91, { size: 24, weight: 500, color: C.muted });
    // Top-right: chapter progress ticks.
    const n = scenes.length;
    const tickW = 26, gap = 8;
    const x0 = W - 110 - n * tickW - (n - 1) * gap;
    for (let i = 0; i < n; i++) {
      const done = i < index ? 1 : i > index ? 0 : loc.local / s.duration;
      g.box(x0 + i * (tickW + gap), 80, tickW, 6, { fill: C.lineSoft, radius: 3 });
      if (done > 0) g.box(x0 + i * (tickW + gap), 80, tickW * done, 6, { fill: i === index ? C.teal : C.dim, radius: 3 });
    }
  }

  function drawCaption(g, s, local) {
    const cap = captionAt(s, local);
    if (!cap) return;
    const a = Math.min(clamp01((local - cap.start) / 0.25), clamp01((cap.end - local) / 0.25));
    if (a <= 0) return;
    const o = { size: 34, weight: 500, font: 'body' };
    const lines = g.wrap(cap.text, 1480, o);
    const lh = 46;
    const boxH = lines.length * lh + 34;
    const widest = Math.max(...lines.map((l) => g.measure(l, o)));
    const boxW = widest + 64;
    const top = CAPTION_Y + 56 - boxH / 2 - (lines.length - 1) * 4;
    g.alpha(a, () => {
      g.box(W / 2 - boxW / 2, top, boxW, boxH, { fill: 'rgba(6,14,16,0.82)', radius: 14 });
      lines.forEach((l, i) => g.text(l, W / 2, top + 47 + i * lh, Object.assign({ align: 'center', color: C.ink }, o)));
    });
  }

  const FADE = 0.45;
  const END_FADE = 0.8; // the whole frame fades to the background at the very end
  let layer = null;

  function renderAt(canvas, t, opts) {
    opts = opts || {};
    const ctx = canvas.getContext('2d');
    const total = duration();
    t = Math.max(0, Math.min(total - 1e-6, t));
    const loc = locate(t);
    const g = new Graphics(ctx);
    ctx.save();
    ctx.setTransform(canvas.width / W, 0, 0, canvas.height / H, 0, 0);
    drawBackground(ctx);
    if (!loc) {
      ctx.restore();
      return;
    }
    const s = loc.scene;
    g.t = loc.local;
    g.d = s.duration;
    drawChrome(g, loc, total);
    // Fade scene content in and out at boundaries (not at the very start,
    // so the first frame is never blank, and not at the very end).
    let a = 1;
    if (loc.index > 0) a = Math.min(a, clamp01(loc.local / FADE));
    if (loc.index < scenes.length - 1) a = Math.min(a, clamp01((s.duration - loc.local) / FADE));
    if (a >= 0.999) {
      ctx.save();
      s.draw(g, loc.local, s.duration);
      ctx.restore();
    } else if (a > 0.001) {
      // Draw the scene at full opacity offscreen, then composite it with the
      // fade alpha; fading each draw call would let lines show through fills.
      if (!layer || layer.width !== canvas.width || layer.height !== canvas.height) {
        layer = document.createElement('canvas');
        layer.width = canvas.width;
        layer.height = canvas.height;
      }
      const lctx = layer.getContext('2d');
      lctx.setTransform(1, 0, 0, 1, 0, 0);
      lctx.clearRect(0, 0, layer.width, layer.height);
      lctx.setTransform(canvas.width / W, 0, 0, canvas.height / H, 0, 0);
      const lg = new Graphics(lctx);
      lg.t = loc.local;
      lg.d = s.duration;
      lctx.save();
      s.draw(lg, loc.local, s.duration);
      lctx.restore();
      ctx.save();
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.globalAlpha = a;
      ctx.drawImage(layer, 0, 0);
      ctx.restore();
    }
    if (opts.captions !== false) drawCaption(g, s, loc.local);
    if (loc.index === scenes.length - 1) {
      const end = clamp01((s.duration - loc.local) / END_FADE);
      if (end < 1) {
        ctx.save();
        ctx.globalAlpha = 1 - end;
        drawBackground(ctx);
        ctx.restore();
      }
    }
    if (opts.debug) {
      g.box(SAFE.x, SAFE.y, SAFE.w, SAFE.h, { stroke: 'rgba(255,0,255,0.6)', dash: [10, 8], radius: 0 });
      g.text(`${s.id}  t=${loc.local.toFixed(2)}s`, W - 110, SAFE.y - 12, { size: 18, font: 'mono', color: '#ff66ff', align: 'right' });
    }
    ctx.restore();
  }

  const fontFaces = [
    '800 60px "Bricolage Grotesque"', '700 60px "Bricolage Grotesque"',
    '400 30px "IBM Plex Sans"', '500 30px "IBM Plex Sans"', '600 30px "IBM Plex Sans"',
    '400 30px "IBM Plex Mono"', '500 30px "IBM Plex Mono"', '600 30px "IBM Plex Mono"',
  ];

  // Resolves when web fonts are loaded (or after a timeout, falling back).
  const ready = (document.fonts
    ? Promise.race([
      Promise.all(fontFaces.map((f) => document.fonts.load(f))),
      new Promise((r) => setTimeout(r, 6000)),
    ])
    : Promise.resolve()
  ).then(() => { background = null; });

  window.AGX = {
    W, H, C, F, SAFE, ease, scene, scenes, chapters, duration, locate, captionAt, renderAt, ready,
    Graphics, hash, rgba, mix,
  };
})();

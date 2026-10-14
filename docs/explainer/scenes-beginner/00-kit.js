// Friendly building blocks shared by the beginner scenes. Everything is
// drawn from simple shapes so it stays crisp at any size.
window.BK = (function () {
  'use strict';

  // A person: a figure in a soft circle with a name underneath.
  function person(g, x, y, o) {
    o = o || {};
    const C = g.C;
    const s = o.size || 1;
    const col = o.color || C.blue;
    g.circle(x, y, 56 * s, { fill: g.rgba(col, 0.14), stroke: g.rgba(col, 0.75), lineWidth: 3, glow: o.glow ? g.rgba(col, 0.6) : null });
    g.icon('person', x, y + 4 * s, 70 * s, { color: col });
    if (o.name) g.text(o.name, x, y + 56 * s + 34, { size: o.nameSize || 26, weight: 600, color: C.ink, align: 'center' });
  }

  // A machine that works on code: a robot face in a soft circle.
  function helper(g, x, y, o) {
    o = o || {};
    const C = g.C;
    const s = o.size || 1;
    const col = o.color || C.blue;
    g.circle(x, y, 50 * s, { fill: g.rgba(col, 0.12), stroke: g.rgba(col, 0.7), lineWidth: 3 });
    g.icon(o.icon || 'agent', x, y + 4 * s, 52 * s, { color: col });
    if (o.name) g.text(o.name, x, y + 50 * s + 30, { size: o.nameSize || 22, weight: 500, color: C.muted, align: 'center' });
  }

  // A speech bubble with a small tail pointing down-left (or down-right).
  function bubble(g, x, y, text, o) {
    o = o || {};
    const C = g.C;
    const col = o.color || C.ink;
    const size = o.size || 24;
    const font = o.font || 'body';
    const padX = 22, h = size * 2.1;
    const w = g.measure(text, { size, font, weight: 500 }) + padX * 2;
    const left = o.align === 'right' ? x - w : o.align === 'center' ? x - w / 2 : x;
    g.box(left, y - h, w, h, { fill: o.fill || C.panel2, stroke: g.rgba(col, 0.6), radius: 16, lineWidth: 2 });
    const ctx = g.ctx;
    const tx = o.tail === 'right' ? left + w - 34 : left + 34;
    ctx.save();
    ctx.fillStyle = o.fill || C.panel2;
    ctx.strokeStyle = g.rgba(col, 0.6);
    ctx.lineWidth = 2;
    ctx.beginPath();
    ctx.moveTo(tx - 10, y - 1);
    ctx.lineTo(tx + (o.tail === 'right' ? 12 : -12), y + 16);
    ctx.lineTo(tx + 10, y - 1);
    ctx.fill();
    ctx.stroke();
    ctx.restore();
    g.text(text, left + padX, y - h / 2 + size * 0.36, { size, font, weight: 500, color: col });
    return w;
  }

  // A stack of layers standing for a project's history.
  function stack(g, x, y, o) {
    o = o || {};
    const C = g.C;
    const s = o.scale || 1;
    const col = o.color || C.amber;
    const n = o.layers || 7;
    for (let i = 0; i < n; i++) {
      g.box(x - 60 * s, y - 16 * s - i * 13 * s, 120 * s, 22 * s, {
        fill: g.rgba(col, 0.16 + i * 0.05), stroke: g.rgba(col, 0.85), radius: 5 * s, lineWidth: 1.5,
      });
    }
    if (o.label) g.text(o.label, x, y + 30 * s + 14, { size: o.labelSize || 20, color: C.muted, align: 'center' });
  }

  // A file card. state: 'same' | 'edited' | 'added' | 'removed'.
  function file(g, x, y, name, o) {
    o = o || {};
    const C = g.C;
    const st = o.state || 'same';
    const col = st === 'edited' ? C.amber : st === 'added' ? C.mint : st === 'removed' ? C.coral : C.muted;
    const w = o.w || 190, h = o.h || 58;
    g.box(x - w / 2, y - h / 2, w, h, {
      fill: st === 'same' ? C.panel : g.rgba(col, 0.16), stroke: g.rgba(col, st === 'same' ? 0.5 : 0.9), radius: 10, lineWidth: 2,
      dash: st === 'removed' ? [7, 6] : null,
    });
    g.icon('file', x - w / 2 + 26, y, 26, { color: col, stroke: 9 });
    g.text(name, x - w / 2 + 48, y + 8, { size: o.size || 22, font: 'mono', color: st === 'removed' ? C.coral : C.ink });
  }

  // A folder row: icon, name and an optional note.
  function folder(g, x, y, name, o) {
    o = o || {};
    const C = g.C;
    const col = o.color || C.muted;
    g.icon('folder', x, y - 8, 34, { color: col, stroke: 8 });
    g.text(name, x + 34, y, { size: o.size || 24, font: 'mono', color: o.dim ? C.dim : C.ink });
    if (o.note) g.text(o.note, x + 34 + g.measure(name, { size: o.size || 24, font: 'mono' }) + 18, y, { size: 20, color: o.noteColor || C.muted });
  }

  return { person, helper, bubble, stack, file, folder };
})();

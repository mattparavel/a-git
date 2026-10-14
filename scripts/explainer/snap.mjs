#!/usr/bin/env node
// Render frames of the a-git explainer to PNG for review.
//
//   node scripts/explainer/snap.mjs --scene 04 --times 1,4.5,9 --out /tmp/snaps
//   node scripts/explainer/snap.mjs --scene 04 --sheet 6 --out /tmp/snaps   (contact sheet)
//   node scripts/explainer/snap.mjs --all --sheet 6 --out /tmp/snaps        (one sheet per scene)
//   add --debug to outline the safe area; --no-captions to hide captions;
//   --page beginner.html to render the beginner video instead of index.html.
//
// --scene matches a scene id ("changesets"), its number ("04"), or a prefix.
// Times are scene-local seconds. Prints the files written and any page errors.
import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadPlaywright, openRenderer, decodeDataURL } from './lib.mjs';

const args = process.argv.slice(2);
const opt = (name, def) => {
  const i = args.indexOf('--' + name);
  return i >= 0 ? (args[i + 1] && !args[i + 1].startsWith('--') ? args[i + 1] : true) : def;
};
const out = opt('out', '/tmp/agx-snaps');
mkdirSync(out, { recursive: true });
const renderOpts = { debug: !!opt('debug', false), captions: !opt('no-captions', false) };

const { chromium } = await loadPlaywright();
const browser = await chromium.launch();
try {
  const { page, errors } = await openRenderer(browser, opt('page', 'index.html'));
  const chapters = await page.evaluate(() => AGX.chapters().map(({ id, title, start, duration }) => ({ id, title, start, duration })));
  const pick = (key) => {
    const k = String(key);
    const byNum = /^\d+$/.test(k) ? chapters[parseInt(k, 10) - 1] : null;
    return byNum || chapters.find((c) => c.id === k) || chapters.find((c) => c.id.startsWith(k));
  };
  const targets = opt('all', false) ? chapters : [pick(opt('scene', '1'))];
  if (!targets[0]) throw new Error('no such scene; scenes: ' + chapters.map((c, i) => `${i + 1}:${c.id}`).join(' '));
  for (const ch of targets) {
    const idx = chapters.indexOf(ch) + 1;
    const tag = String(idx).padStart(2, '0') + '-' + ch.id;
    const sheetN = opt('sheet', false);
    if (sheetN) {
      const n = sheetN === true ? 6 : parseInt(sheetN, 10);
      const times = Array.from({ length: n }, (_, i) => ch.start + Math.min(ch.duration - 0.05, (ch.duration * (i + 0.5)) / n));
      const png = await page.evaluate(([ts, o]) => window.sheet(ts, o), [times, renderOpts]);
      const file = join(out, `${tag}-sheet.png`);
      writeFileSync(file, decodeDataURL(png));
      console.log(file, ' times:', times.map((t) => (t - ch.start).toFixed(2)).join(','));
    } else {
      const times = String(opt('times', '1')).split(',').map(Number);
      for (const lt of times) {
        await page.evaluate(([t, o]) => window.renderFrame(t, o), [ch.start + Math.min(lt, ch.duration - 0.01), renderOpts]);
        const png = await page.evaluate(() => window.frameData('image/png'));
        const file = join(out, `${tag}-t${lt.toFixed(2)}.png`);
        writeFileSync(file, decodeDataURL(png));
        console.log(file);
      }
    }
  }
  if (errors.length) {
    console.log('PAGE ERRORS:');
    for (const e of errors) console.log('  ' + e);
    process.exitCode = 1;
  }
} finally {
  await browser.close();
}

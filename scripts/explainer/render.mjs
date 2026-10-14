#!/usr/bin/env node
// Render the a-git explainer to an MP4 (H.264) with headless Chromium + ffmpeg.
//
//   node scripts/explainer/render.mjs [--out docs/explainer/a-git-explainer.mp4]
//        [--fps 30] [--workers 3] [--from 0] [--to <seconds>] [--crf 20]
//
// Each worker renders a contiguous range of frames; the ranges are encoded
// separately and then joined without re-encoding.
import { mkdirSync, writeFileSync, rmSync, existsSync } from 'node:fs';
import { join, resolve } from 'node:path';
import { tmpdir } from 'node:os';
import { spawn } from 'node:child_process';
import { loadPlaywright, openRenderer, decodeDataURL, EXPLAINER_DIR } from './lib.mjs';

const args = process.argv.slice(2);
const opt = (name, def) => {
  const i = args.indexOf('--' + name);
  return i >= 0 ? args[i + 1] : def;
};
const outFile = resolve(opt('out', join(EXPLAINER_DIR, 'a-git-explainer.mp4')));
const fps = parseInt(opt('fps', '30'), 10);
const workers = parseInt(opt('workers', '3'), 10);
const crf = opt('crf', '20');

function run(cmd, argv, input) {
  return new Promise((res, rej) => {
    const p = spawn(cmd, argv, { stdio: [input ? 'pipe' : 'ignore', 'ignore', 'pipe'] });
    let err = '';
    p.stderr.on('data', (d) => { err += d; });
    p.on('error', rej);
    p.on('close', (code) => (code === 0 ? res() : rej(new Error(`${cmd} exited ${code}: ${err.slice(-2000)}`))));
    if (input) input(p.stdin);
  });
}

const { chromium } = await loadPlaywright();
const browser = await chromium.launch();
const work = join(tmpdir(), 'agx-render-' + process.pid);
mkdirSync(work, { recursive: true });
try {
  const probe = await openRenderer(browser);
  const total = await probe.page.evaluate(() => AGX.duration());
  await probe.page.close();
  const from = parseFloat(opt('from', '0'));
  const to = Math.min(total, parseFloat(opt('to', String(total))));
  const frames = Math.round((to - from) * fps);
  console.log(`rendering ${frames} frames (${(to - from).toFixed(1)}s at ${fps} fps) with ${workers} workers`);
  const per = Math.ceil(frames / workers);
  const started = Date.now();
  let done = 0;
  const segments = [];
  await Promise.all(Array.from({ length: workers }, async (_, w) => {
    const first = w * per, last = Math.min(frames, first + per);
    if (first >= last) return;
    const seg = join(work, `seg${w}.mp4`);
    segments[w] = seg;
    const { page, errors } = await openRenderer(browser);
    // Stream PNG frames straight into ffmpeg.
    await run('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'image2pipe', '-framerate', String(fps), '-c:v', 'png', '-i', '-',
      '-c:v', 'libx264', '-preset', 'slow', '-crf', crf, '-pix_fmt', 'yuv420p', '-tune', 'animation', seg], async (stdin) => {
      for (let f = first; f < last; f++) {
        const t = from + f / fps;
        const url = await page.evaluate((tt) => { window.renderFrame(tt, {}); return window.frameData('image/png'); }, t);
        if (!stdin.write(decodeDataURL(url))) await new Promise((r) => stdin.once('drain', r));
        done++;
        if (done % (fps * 10) === 0) {
          const el = (Date.now() - started) / 1000;
          console.log(`  ${done}/${frames} frames, ${el.toFixed(0)}s elapsed, ~${((frames - done) * el / done).toFixed(0)}s left`);
        }
      }
      stdin.end();
    });
    if (errors.length) throw new Error('page errors: ' + errors.join('; '));
    await page.close();
  }));
  const list = join(work, 'list.txt');
  writeFileSync(list, segments.filter(Boolean).map((s) => `file '${s}'`).join('\n'));
  await run('ffmpeg', ['-y', '-loglevel', 'error', '-f', 'concat', '-safe', '0', '-i', list, '-c', 'copy', '-movflags', '+faststart', outFile]);
  console.log(`wrote ${outFile} in ${((Date.now() - started) / 1000).toFixed(0)}s`);
} finally {
  await browser.close();
  if (existsSync(work)) rmSync(work, { recursive: true, force: true });
}

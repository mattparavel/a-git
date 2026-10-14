// Shared helpers for rendering the a-git explainer headlessly.
import { createRequire } from 'node:module';
import { readFileSync, writeFileSync, mkdtempSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join, dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));
export const EXPLAINER_DIR = resolve(here, '../../docs/explainer');

// Load Playwright from the project, a global install, or the preinstalled
// copy in this environment.
export async function loadPlaywright() {
  const candidates = ['playwright', '/opt/node-tools/node_modules/playwright/index.js'];
  const require = createRequire(import.meta.url);
  for (const c of candidates) {
    try {
      return require(c);
    } catch {}
  }
  throw new Error('Playwright not found: npm install playwright (or set NODE_PATH)');
}

// Builds a standalone render page that loads the same scripts, in the same
// order, as the player page (docs/explainer/index.html).
export function renderPageFile() {
  const html = readFileSync(join(EXPLAINER_DIR, 'index.html'), 'utf8');
  const fonts = (html.match(/<link rel="stylesheet" href="([^"]+)"/) || [])[1];
  const scripts = [...html.matchAll(/<script src="([^"]+)"><\/script>/g)].map((m) =>
    pathToFileURL(join(EXPLAINER_DIR, m[1])).href,
  );
  const page = `<!doctype html><html><head><meta charset="utf-8">
<link rel="stylesheet" href="${fonts}">
<style>html,body{margin:0;background:#000}canvas{display:block;width:1920px;height:1080px}</style>
</head><body><canvas id="stage" width="1920" height="1080"></canvas>
${scripts.map((s) => `<script src="${s}"></script>`).join('\n')}
<script>
window.renderFrame = (t, opts) => { AGX.renderAt(document.getElementById('stage'), t, opts || {}); };
window.frameData = (type, q) => document.getElementById('stage').toDataURL(type || 'image/png', q);
window.sheet = (times, opts) => {
  // Contact sheet: frames at the given absolute times, 2 columns at half size.
  const stage = document.getElementById('stage');
  const cols = 2, w = 960, h = 540, pad = 8, label = 30;
  const rows = Math.ceil(times.length / cols);
  const c = document.createElement('canvas');
  c.width = cols * w + (cols + 1) * pad;
  c.height = rows * (h + label) + (rows + 1) * pad;
  const x = c.getContext('2d');
  x.fillStyle = '#222'; x.fillRect(0, 0, c.width, c.height);
  times.forEach((t, i) => {
    AGX.renderAt(stage, t, opts || {});
    const cx = pad + (i % cols) * (w + pad), cy = pad + Math.floor(i / cols) * (h + label + pad);
    x.fillStyle = '#ddd'; x.font = '20px monospace';
    const loc = AGX.locate(t);
    x.fillText(loc.scene.id + '  local t=' + loc.local.toFixed(2) + 's  (abs ' + t.toFixed(2) + 's)', cx + 4, cy + 22);
    x.drawImage(stage, cx, cy + label, w, h);
  });
  return c.toDataURL('image/png');
};
window.__ready = AGX.ready.then(() => true);
</script></body></html>`;
  const dir = mkdtempSync(join(tmpdir(), 'agx-'));
  const file = join(dir, 'render.html');
  writeFileSync(file, page);
  return file;
}

export async function openRenderer(browser) {
  const page = await browser.newPage({ viewport: { width: 1920, height: 1080 }, deviceScaleFactor: 1 });
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  page.on('console', (m) => { if (m.type() === 'error') errors.push(m.text()); });
  await page.goto(pathToFileURL(renderPageFile()).href);
  await page.evaluate(() => window.__ready);
  return { page, errors };
}

export function decodeDataURL(url) {
  return Buffer.from(url.slice(url.indexOf(',') + 1), 'base64');
}

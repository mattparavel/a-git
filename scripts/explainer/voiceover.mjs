#!/usr/bin/env node
// Add an ElevenLabs voiceover to an explainer video.
//
//   node scripts/explainer/voiceover.mjs --page beginner.html
//
// The API key comes from ELEVENLABS_API_KEY, or from a network secret that
// the environment's proxy attaches to requests for api.elevenlabs.io (header
// xi-api-key, no prefix). In that case the key never reaches this process.
//
// Options:
//   --page   beginner.html | index.html        which explainer to narrate
//   --video  <mp4>     rendered video to add the voice to
//                      (default: the page's video in docs/explainer/)
//   --out    <mp4>     output (default: <video name>-voiced.mp4)
//   --voice  Chris     ElevenLabs voice name or voice ID
//   --model  eleven_multilingual_v2
//   --speed  1.0       ElevenLabs speaking speed (0.7 - 1.2)
//   --plan             print each caption's time window and stop (no API calls)
//
// Each caption becomes one clip, placed where that caption appears on screen.
// Clips are cached by text, voice, model and speed in
// docs/explainer/voiceover/<page>/, so re-running after a caption edit only
// pays for the changed lines. A clip that is longer than its caption's time
// window is sped up slightly (up to 12%); anything longer is reported so the
// scene timing can be loosened.
import { createHash } from 'node:crypto';
import { existsSync, mkdirSync, writeFileSync, readFileSync } from 'node:fs';
import { join, resolve, basename } from 'node:path';
import { spawn, spawnSync } from 'node:child_process';
import { loadPlaywright, openRenderer, EXPLAINER_DIR } from './lib.mjs';

// Node's fetch ignores HTTPS_PROXY unless NODE_USE_ENV_PROXY is set
// (Node 22.21 and later), so re-run under it when a proxy is configured.
if ((process.env.HTTPS_PROXY || process.env.https_proxy) && !process.env.NODE_USE_ENV_PROXY) {
  const r = spawnSync(process.execPath, [...process.execArgv, ...process.argv.slice(1)], {
    stdio: 'inherit',
    env: { ...process.env, NODE_USE_ENV_PROXY: '1', NODE_NO_WARNINGS: '1' },
  });
  process.exit(r.status ?? 1);
}

const args = process.argv.slice(2);
const opt = (name, def) => {
  const i = args.indexOf('--' + name);
  if (i < 0) return def;
  const v = args[i + 1];
  return v && !v.startsWith('--') ? v : true;
};

const page = opt('page', 'beginner.html');
const pageBase = basename(page, '.html');
const defaultVideo = page === 'index.html' ? 'a-git-explainer.mp4' : 'a-git-for-beginners.mp4';
const video = resolve(opt('video', join(EXPLAINER_DIR, defaultVideo)));
const out = resolve(opt('out', video.replace(/\.mp4$/, '-voiced.mp4')));
const voiceName = String(opt('voice', 'Chris'));
const model = String(opt('model', 'eleven_multilingual_v2'));
const speed = parseFloat(opt('speed', '1.0'));
const planOnly = !!opt('plan', false);
const outDir = join(EXPLAINER_DIR, 'voiceover', pageBase);
const API = 'https://api.elevenlabs.io/v1';
const GAP = 0.25;        // silence kept before the next caption starts
const MAX_TEMPO = 1.12;  // how much a clip may be sped up to fit its window

function run(cmd, argv, opts = {}) {
  return new Promise((res, rej) => {
    const p = spawn(cmd, argv, { stdio: ['ignore', 'pipe', 'pipe'] });
    let so = '', se = '';
    p.stdout.on('data', (d) => { so += d; });
    p.stderr.on('data', (d) => { se += d; });
    p.on('error', rej);
    p.on('close', (code) => (code === 0 || opts.allowFail ? res(so) : rej(new Error(`${cmd} exited ${code}: ${se.slice(-1500)}`))));
  });
}

async function duration(file) {
  const s = await run('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', file]);
  return parseFloat(s.trim());
}

// 1. Read the captions and their time windows from the player page.
const { chromium } = await loadPlaywright();
const browser = await chromium.launch();
let lines = [];
let total = 0;
try {
  const { page: tab } = await openRenderer(browser, page);
  const chapters = await tab.evaluate(() => AGX.chapters());
  total = await tab.evaluate(() => AGX.duration());
  for (const ch of chapters) {
    ch.captions.forEach((c, i) => {
      const next = i + 1 < ch.captions.length ? ch.captions[i + 1].at : ch.duration;
      lines.push({ scene: ch.id, start: ch.start + c.at, window: next - c.at - GAP, text: c.text });
    });
  }
} finally {
  await browser.close();
}

if (planOnly) {
  for (const l of lines) console.log(`${l.start.toFixed(2).padStart(7)}s  ${l.window.toFixed(1).padStart(5)}s  ${l.scene.padEnd(10)} ${l.text}`);
  console.log(`${lines.length} lines, ${lines.reduce((n, l) => n + l.text.length, 0)} characters, video ${total.toFixed(1)}s`);
  process.exit(0);
}

const key = process.env.ELEVENLABS_API_KEY;
if (!key) console.log('ELEVENLABS_API_KEY is not set; relying on a network secret for api.elevenlabs.io.');
if (!existsSync(video)) {
  console.error(`video not found: ${video} (render it first with scripts/explainer/render.mjs)`);
  process.exit(2);
}
mkdirSync(outDir, { recursive: true });

// 2. Find the voice by name (or accept a voice ID).
async function api(path, init = {}) {
  const headers = { ...(key ? { 'xi-api-key': key } : {}), ...(init.headers || {}) };
  let res;
  try {
    res = await fetch(API + path, { ...init, headers });
  } catch (e) {
    console.error(`Cannot reach ${API} (${e.cause?.message || e.message}). If a proxy is in use, the network policy may block`
      + ' api.elevenlabs.io: allow it, or add the key as a network secret for that host (see docs/explainer/README.md).');
    process.exit(2);
  }
  if (res.status === 401 && !key) {
    console.error('ElevenLabs rejected the request as unauthenticated. Set ELEVENLABS_API_KEY, or add a network secret'
      + ' for api.elevenlabs.io with the header xi-api-key and no prefix.');
    process.exit(2);
  }
  if (!res.ok) {
    const body = await res.text().catch(() => '');
    throw new Error(`ElevenLabs ${init.method || 'GET'} ${path}: HTTP ${res.status} ${body.slice(0, 400)}`);
  }
  return res;
}

let voiceId = voiceName;
if (!/^[A-Za-z0-9]{20}$/.test(voiceName)) {
  const res = await api('/voices');
  const { voices } = await res.json();
  const match = voices.find((v) => v.name.toLowerCase() === voiceName.toLowerCase())
    || voices.find((v) => v.name.toLowerCase().startsWith(voiceName.toLowerCase()));
  if (!match) {
    console.error(`No voice named "${voiceName}". Available: ${voices.map((v) => v.name).join(', ')}`);
    process.exit(2);
  }
  voiceId = match.voice_id;
  console.log(`voice: ${match.name} (${voiceId})`);
}

// 3. Synthesize each line (cached), passing neighbouring lines for natural flow.
const settings = { stability: 0.5, similarity_boost: 0.75, style: 0.15, use_speaker_boost: true, speed };
for (let i = 0; i < lines.length; i++) {
  const l = lines[i];
  const id = createHash('sha1').update(JSON.stringify([voiceId, model, settings, l.text])).digest('hex').slice(0, 16);
  l.file = join(outDir, `${String(i + 1).padStart(2, '0')}-${id}.mp3`);
  if (!existsSync(l.file)) {
    const res = await api(`/text-to-speech/${voiceId}?output_format=mp3_44100_128`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Accept: 'audio/mpeg' },
      body: JSON.stringify({
        text: l.text,
        model_id: model,
        voice_settings: settings,
        previous_text: i > 0 ? lines[i - 1].text : undefined,
        next_text: i + 1 < lines.length ? lines[i + 1].text : undefined,
      }),
    });
    writeFileSync(l.file, Buffer.from(await res.arrayBuffer()));
    console.log(`  synthesized ${basename(l.file)}`);
  }
  l.duration = await duration(l.file);
  l.tempo = l.duration > l.window ? l.duration / l.window : 1;
}

// 4. Report fit.
const tooLong = lines.filter((l) => l.tempo > MAX_TEMPO);
for (const l of lines) {
  const flag = l.tempo > MAX_TEMPO ? '  TOO LONG' : l.tempo > 1 ? `  sped up ${((l.tempo - 1) * 100).toFixed(0)}%` : '';
  console.log(`${l.start.toFixed(2).padStart(7)}s  voice ${l.duration.toFixed(1)}s / window ${l.window.toFixed(1)}s${flag}  ${l.scene}`);
}
if (tooLong.length) {
  console.error(`\n${tooLong.length} line(s) do not fit even at ${MAX_TEMPO}x. Give those captions more time (later next caption or longer scene), re-render, and run again.`);
  process.exit(1);
}

// 5. Mix all clips into one narration track, then add it to the video.
const narration = join(outDir, 'narration.m4a');
const inputs = [];
const filters = [];
lines.forEach((l, i) => {
  inputs.push('-i', l.file);
  const ms = Math.round(l.start * 1000);
  const tempo = l.tempo > 1 ? `atempo=${l.tempo.toFixed(4)},` : '';
  filters.push(`[${i}:a]${tempo}aresample=48000,adelay=${ms}|${ms}[a${i}]`);
});
filters.push(`${lines.map((_, i) => `[a${i}]`).join('')}amix=inputs=${lines.length}:normalize=0:dropout_transition=0,apad=whole_dur=${total.toFixed(3)},loudnorm=I=-16:TP=-1.5:LRA=11[mix]`);
await run('ffmpeg', ['-y', '-loglevel', 'error', ...inputs, '-filter_complex', filters.join(';'), '-map', '[mix]', '-t', total.toFixed(3), '-c:a', 'aac', '-b:a', '160k', narration]);
await run('ffmpeg', ['-y', '-loglevel', 'error', '-i', video, '-i', narration, '-map', '0:v', '-map', '1:a', '-c:v', 'copy', '-c:a', 'copy', '-shortest', '-movflags', '+faststart', out]);

writeFileSync(join(outDir, 'manifest.json'), JSON.stringify({
  page, voice: voiceName, voiceId, model, settings, narration: basename(narration),
  lines: lines.map((l) => ({ start: +l.start.toFixed(3), duration: +l.duration.toFixed(3), tempo: +l.tempo.toFixed(4), file: basename(l.file), text: l.text })),
}, null, 2) + '\n');
console.log(`\nwrote ${out}\nnarration track: ${narration}`);

// Shared player for the explainer pages: playback, scrubbing, chapters,
// captions toggle and transcript. Loaded after engine.js and the scenes.
(function () {
  'use strict';
  const AGX = window.AGX;
  const canvas = document.getElementById('stage');
  const screen = document.getElementById('screen');
  const playBtn = document.getElementById('play');
  const playIcon = document.getElementById('playIcon');
  const bigPlay = document.getElementById('bigPlay');
  const timeEl = document.getElementById('time');
  const scrub = document.getElementById('scrub');
  const ticks = document.getElementById('ticks');
  const ccBtn = document.getElementById('cc');
  const speedSel = document.getElementById('speed');
  const fsBtn = document.getElementById('fs');
  const list = document.getElementById('chapterList');
  const transcript = document.getElementById('transcript');

  const total = AGX.duration();
  const chapters = AGX.chapters();
  // A frame where the title is fully drawn, shown before playback starts.
  const POSTER = parseFloat(canvas.dataset.poster || '4.2');

  let t = POSTER;
  let playing = false;
  let started = false;
  let captions = true;
  let speed = 1;
  let last = 0;

  function fmt(s) {
    s = Math.max(0, Math.floor(s));
    return Math.floor(s / 60) + ':' + String(s % 60).padStart(2, '0');
  }

  document.getElementById('runtime').innerHTML = '<b>' + fmt(total) + '</b> runtime';
  scrub.max = String(total);

  // Chapter list, scrubber ticks and transcript.
  const chapterButtons = [];
  const captionEls = [];
  chapters.forEach((c) => {
    const li = document.createElement('li');
    const b = document.createElement('button');
    b.type = 'button';
    b.innerHTML = '<span class="n">' + String(c.index + 1).padStart(2, '0') + '</span><span></span><span class="ts">' + fmt(c.start) + '</span>';
    b.children[1].textContent = c.title;
    b.addEventListener('click', () => seek(c.start + 0.01, true));
    li.appendChild(b);
    list.appendChild(li);
    chapterButtons.push(b);

    if (c.index > 0) {
      const tick = document.createElement('i');
      tick.style.left = (100 * c.start / total) + '%';
      ticks.appendChild(tick);
    }

    const sec = document.createElement('section');
    const h = document.createElement('h3');
    h.innerHTML = '<span></span><span class="ts">' + fmt(c.start) + '</span>';
    h.firstChild.textContent = c.title;
    sec.appendChild(h);
    c.captions.forEach((cap) => {
      const p = document.createElement('p');
      p.textContent = cap.text;
      sec.appendChild(p);
      captionEls.push({ el: p, start: c.start + cap.at });
    });
    transcript.appendChild(sec);
  });

  function render() {
    AGX.renderAt(canvas, t, { captions });
    timeEl.textContent = fmt(t) + ' / ' + fmt(total);
    scrub.value = String(t);
    const loc = AGX.locate(t);
    chapterButtons.forEach((b, i) => b.classList.toggle('on', loc && i === loc.index));
    let cur = -1;
    captionEls.forEach((c, i) => { if (t >= c.start) cur = i; });
    captionEls.forEach((c, i) => c.el.classList.toggle('on', i === cur && started));
  }

  function setPlaying(p) {
    playing = p;
    playIcon.setAttribute('d', p ? 'M6 4h4v16H6zM14 4h4v16h-4z' : 'M5 3l16 9-16 9z');
    playBtn.setAttribute('aria-label', p ? 'Pause' : 'Play');
    bigPlay.hidden = p || started;
    if (p) {
      if (!started) { started = true; t = 0; }
      if (t >= total - 0.05) t = 0;
      last = performance.now();
      requestAnimationFrame(tick);
    }
  }

  function tick(now) {
    if (!playing) return;
    const dt = Math.min(0.1, (now - last) / 1000);
    last = now;
    t += dt * speed;
    if (t >= total) { t = total; render(); setPlaying(false); return; }
    render();
    requestAnimationFrame(tick);
  }

  function seek(to, play) {
    started = true;
    bigPlay.hidden = true;
    t = Math.max(0, Math.min(total, to));
    render();
    if (play && !playing) setPlaying(true);
  }

  playBtn.addEventListener('click', () => setPlaying(!playing));
  bigPlay.addEventListener('click', () => setPlaying(true));
  canvas.addEventListener('click', () => setPlaying(!playing));
  scrub.addEventListener('input', () => seek(parseFloat(scrub.value), false));
  ccBtn.addEventListener('click', () => {
    captions = !captions;
    ccBtn.setAttribute('aria-pressed', String(captions));
    render();
  });
  speedSel.addEventListener('change', () => { speed = parseFloat(speedSel.value) || 1; });
  fsBtn.addEventListener('click', () => {
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {});
    } else if (screen.requestFullscreen) {
      screen.requestFullscreen().catch(() => {});
    }
  });
  document.addEventListener('keydown', (e) => {
    if (e.target && (e.target.tagName === 'INPUT' && e.target.type !== 'range' || e.target.tagName === 'SELECT')) return;
    if (e.key === ' ' || e.key === 'k') {
      if (e.target && e.target.tagName === 'BUTTON' && e.key === ' ') return;
      e.preventDefault();
      setPlaying(!playing);
    } else if (e.key === 'ArrowRight') { e.preventDefault(); seek(t + 5, false); }
    else if (e.key === 'ArrowLeft') { e.preventDefault(); seek(t - 5, false); }
    else if (e.key === 'c' || e.key === 'C') { ccBtn.click(); }
    else if (e.key === 'f' || e.key === 'F') { fsBtn.click(); }
  });

  // Keep the playhead across live updates of the page.
  const hot = window.claude && window.claude.hot;
  if (hot && hot.snapshot) hot.snapshot(() => ({ t, started }));
  const restore = (data) => {
    if (data && typeof data.t === 'number') { t = data.t; started = !!data.started; bigPlay.hidden = started; }
  };
  if (hot && hot.data) restore(hot.data);

  AGX.ready.then(render);
  render();
})();

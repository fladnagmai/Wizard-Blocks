// The portal: the original pixel wizard falls into a swirling portal, over
// and over. Click him to enter the pools (the BCH page first).
import { WIZARD_BODY, WIZARD_KEYS, WIZARD_LEGS } from './sprites.js';

const PX = 4; // screen pixels per art pixel
const cv = document.getElementById('scene');
const ctx = cv.getContext('2d');
const still = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
let W = 0, H = 0, stars = [], BIG = 2; // BIG: the wizard's size in art pixels

const hex = (n) => '#' + n.toString(16).padStart(6, '0');
// The wizard (body + jumping legs), drawn once per robe colour step.
const rows = [...WIZARD_BODY, ...WIZARD_LEGS.jump];
const SW = rows[0].length, SH = rows.length;
const ROBE = ['#ff4d6d', '#ff9e3d', '#ffe066', '#5ce07a', '#3fb7ff', '#8a5cff', '#e45cff'];
const frames = ROBE.map((_, k) => {
  const c = document.createElement('canvas');
  c.width = SW; c.height = SH;
  const g = c.getContext('2d');
  rows.forEach((row, y) => [...row].forEach((ch, x) => {
    if (ch === '.') return;
    g.fillStyle = ch === 'R' ? ROBE[(k + Math.floor((x + y) / 3)) % ROBE.length] : hex(WIZARD_KEYS[ch] ?? 0xff00ff);
    g.fillRect(x, y, 1, 1);
  }));
  return c;
});

function resize() {
  W = Math.ceil(window.innerWidth / PX); H = Math.ceil(window.innerHeight / PX);
  cv.width = W; cv.height = H;
  ctx.imageSmoothingEnabled = false;
  BIG = W >= 200 ? 2.5 : 1.75;
  stars = Array.from({ length: Math.round(W * H / 90) }, () => ({ x: Math.random() * W | 0, y: Math.random() * H | 0, p: Math.random() * 6.28 }));
}

// One fall lasts FALL s, then a short pause before he falls again.
const FALL = 3.2, PAUSE = 0.7;
let wiz = { x: 0, y: 0, s: 1, r: 0, show: false };

function portalAt() {
  return { x: W / 2, y: H * 0.64, rx: Math.max(22, Math.min(W * 0.28, 70)), ry: Math.max(8, Math.min(H * 0.07, 22)) };
}

function draw(t) {
  ctx.fillStyle = '#000';
  ctx.fillRect(0, 0, W, H);
  for (const s of stars) {
    const a = 0.25 + 0.25 * Math.sin(t * 1.7 + s.p);
    ctx.fillStyle = `rgba(200, 180, 255, ${a})`;
    ctx.fillRect(s.x, s.y, 1, 1);
  }
  const p = portalAt();
  // Swirling rings, outside in; the back half is drawn before the wizard
  // and the front half after him, so he falls into it.
  const ring = (front) => {
    for (let i = 0; i < 9; i++) {
      const k = 1 - i / 9;
      const hue = (260 + i * 14 + t * 60) % 360;
      ctx.strokeStyle = `hsl(${hue}, 90%, ${45 + i * 5}%)`;
      ctx.lineWidth = 1;
      ctx.beginPath();
      const spin = t * (1.2 + i * 0.15);
      ctx.ellipse(Math.round(p.x), Math.round(p.y), p.rx * k, p.ry * k, 0, front ? spin : spin + Math.PI, front ? spin + Math.PI : spin + 2 * Math.PI);
      ctx.stroke();
    }
    if (!front) {
      ctx.fillStyle = '#05010d';
      ctx.beginPath();
      ctx.ellipse(p.x, p.y, p.rx * 0.18, p.ry * 0.18, 0, 0, Math.PI * 2);
      ctx.fill();
    }
  };
  ring(false);
  // The wizard: falls from above the screen, spins, and shrinks away into
  // the portal's centre.
  const cyc = t % (FALL + PAUSE), f = Math.min(1, cyc / FALL);
  wiz.show = cyc < FALL;
  if (wiz.show) {
    const e = f * (0.6 + 0.4 * f); // falling a little faster as he goes
    wiz.x = p.x + Math.sin(f * 7) * (1 - f) * 10;
    wiz.y = -SH * BIG + (p.y + SH * BIG) * e;
    wiz.s = f < 0.7 ? 1 : Math.max(0, 1 - (f - 0.7) / 0.3);
    wiz.r = f * f * Math.PI * 3;
    ctx.save();
    ctx.translate(Math.round(wiz.x), Math.round(wiz.y));
    ctx.rotate(wiz.r);
    ctx.scale(wiz.s * BIG, wiz.s * BIG);
    ctx.drawImage(frames[Math.floor(t * 8) % frames.length], -SW / 2, -SH / 2);
    ctx.restore();
  }
  ring(true);
}

let raf = 0;
function frame(ts) {
  draw(ts / 1000);
  raf = requestAnimationFrame(frame);
}

// Click (or tap) on the wizard. He is small and moving, so the hit box is
// generous; the portal counts too while he is falling into it.
function hit(ev) {
  const x = ev.clientX / PX, y = ev.clientY / PX, p = portalAt();
  const nearWiz = wiz.show && wiz.s > 0.2 && Math.abs(x - wiz.x) < SW * BIG && Math.abs(y - wiz.y) < SH * BIG;
  const inPortal = ((x - p.x) / p.rx) ** 2 + ((y - p.y) / p.ry) ** 2 <= 1.2;
  return nearWiz || inPortal;
}
cv.addEventListener('mousemove', (ev) => cv.classList.toggle('over', hit(ev)));
cv.addEventListener('click', (ev) => { if (hit(ev)) location.href = 'bch/'; });

window.addEventListener('resize', resize);
resize();
if (still) {
  draw(FALL * 0.55); // a still frame: the wizard halfway down
} else {
  raf = requestAnimationFrame(frame);
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) { cancelAnimationFrame(raf); raf = 0; } else if (!raf) raf = requestAnimationFrame(frame);
  });
}

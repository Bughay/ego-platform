/* ── Animated orb background ── */
(function () {
  const canvas = document.getElementById('bg');
  const ctx = canvas.getContext('2d');
  let W, H, orbs;

  function resize() {
    W = canvas.width  = window.innerWidth;
    H = canvas.height = window.innerHeight;
  }

  function makeOrbs() {
    orbs = Array.from({ length: 6 }, () => ({
      x: Math.random() * W,
      y: Math.random() * H,
      r: 120 + Math.random() * 180,
      dx: (Math.random() - 0.5) * 0.4,
      dy: (Math.random() - 0.5) * 0.4,
      hue: Math.random() < 0.5 ? 185 : 300,
    }));
  }

  function draw() {
    ctx.clearRect(0, 0, W, H);

    const bg = ctx.createRadialGradient(W / 2, H / 2, 0, W / 2, H / 2, Math.max(W, H) * 0.8);
    bg.addColorStop(0, '#0a0e1f');
    bg.addColorStop(1, '#040d14');
    ctx.fillStyle = bg;
    ctx.fillRect(0, 0, W, H);

    for (const o of orbs) {
      const g = ctx.createRadialGradient(o.x, o.y, 0, o.x, o.y, o.r);
      g.addColorStop(0,   `hsla(${o.hue}, 100%, 60%, 0.12)`);
      g.addColorStop(0.5, `hsla(${o.hue}, 100%, 50%, 0.04)`);
      g.addColorStop(1,   `hsla(${o.hue}, 100%, 40%, 0)`);
      ctx.beginPath();
      ctx.arc(o.x, o.y, o.r, 0, Math.PI * 2);
      ctx.fillStyle = g;
      ctx.fill();

      o.x += o.dx;
      o.y += o.dy;
      if (o.x < -o.r)    o.x = W + o.r;
      if (o.x > W + o.r) o.x = -o.r;
      if (o.y < -o.r)    o.y = H + o.r;
      if (o.y > H + o.r) o.y = -o.r;
    }

    requestAnimationFrame(draw);
  }

  resize();
  makeOrbs();
  draw();
  window.addEventListener('resize', () => { resize(); makeOrbs(); });
})();

/* ── Auth guard + operator identity ── */
API.requireAuth();
(function () {
  const user = API.getUser();
  const el = document.getElementById('username');
  if (el && user && user.email) el.textContent = user.email;
})();

/* ── Logout ── */
document.getElementById('btnLogout').addEventListener('click', function () {
  API.logout();
});

/* ── Launch handlers ── */
document.querySelectorAll('.launch-btn').forEach(function (btn) {
  btn.addEventListener('click', function () {
    const app = this.dataset.app;
    if (app === 'EGOLIFTER') {
      window.location.href = '../egolifter/';
    } else if (app === 'EGO_AI_STUDIO') {
      window.location.href = '../ego_ai_studio/';
    }
  });
});

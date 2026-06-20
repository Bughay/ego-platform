/* EgoLifter landing page animated background and light interactions. */

const prefersReduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;

/* ── Animated orb background (cyan and magenta) ── */
(function () {
  const canvas = document.getElementById('bg');
  if (!canvas) return;
  const ctx = canvas.getContext('2d');
  let W, H, orbs;

  function resize() {
    W = canvas.width = window.innerWidth;
    H = canvas.height = window.innerHeight;
  }

  function makeOrbs() {
    orbs = Array.from({ length: 6 }, () => ({
      x: Math.random() * W,
      y: Math.random() * H,
      r: 120 + Math.random() * 200,
      dx: (Math.random() - 0.5) * 0.4,
      dy: (Math.random() - 0.5) * 0.4,
      hue: Math.random() < 0.5 ? 185 : 300,
    }));
  }

  function paint(move) {
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

      if (move) {
        o.x += o.dx;
        o.y += o.dy;
        if (o.x < -o.r)    o.x = W + o.r;
        if (o.x > W + o.r) o.x = -o.r;
        if (o.y < -o.r)    o.y = H + o.r;
        if (o.y > H + o.r) o.y = -o.r;
      }
    }
  }

  function loop() {
    paint(true);
    requestAnimationFrame(loop);
  }

  resize();
  makeOrbs();
  window.addEventListener('resize', () => { resize(); makeOrbs(); if (prefersReduced) paint(false); });

  if (prefersReduced) {
    paint(false); // single static frame
  } else {
    loop();
  }
})();

/* ── Sticky nav state + mobile toggle ── */
(function () {
  const nav = document.getElementById('nav');
  const toggle = document.getElementById('navToggle');
  const links = document.getElementById('navLinks');

  // Sections are full height and snap flush to the top, so no scroll offset.
  document.documentElement.style.scrollPaddingTop = '0px';

  function onScroll() {
    if (!nav) return;
    nav.classList.toggle('scrolled', window.scrollY > 40);
  }
  onScroll();
  window.addEventListener('scroll', onScroll, { passive: true });

  if (toggle && links) {
    toggle.addEventListener('click', () => {
      const open = links.classList.toggle('open');
      toggle.setAttribute('aria-expanded', String(open));
    });
    // Close the mobile menu after tapping any link.
    links.querySelectorAll('a').forEach(a => {
      a.addEventListener('click', () => {
        links.classList.remove('open');
        toggle.setAttribute('aria-expanded', 'false');
      });
    });
  }
})();

/* ── Scroll reveal ── */
(function () {
  const items = document.querySelectorAll('.reveal');
  if (prefersReduced || !('IntersectionObserver' in window)) {
    items.forEach(el => el.classList.add('visible'));
    return;
  }

  // Toggle (not just add) so content animates in each time its section becomes
  // the active panel, and replays when scrolling back to it.
  const observer = new IntersectionObserver((entries) => {
    entries.forEach(entry => {
      entry.target.classList.toggle('visible', entry.isIntersecting);
    });
  }, { threshold: 0.25 });

  items.forEach(el => observer.observe(el));
})();

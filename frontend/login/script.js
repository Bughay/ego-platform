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

/* ── Form validation ── */
const form       = document.getElementById('loginForm');
const emailEl    = document.getElementById('email');
const passwordEl = document.getElementById('password');

function setError(fieldId, show) {
  document.getElementById(fieldId).classList.toggle('has-error', show);
}

function validateEmail(v) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v.trim());
}

emailEl.addEventListener('input', () => {
  if (document.getElementById('field-email').classList.contains('has-error')) {
    setError('field-email', !validateEmail(emailEl.value));
  }
});

passwordEl.addEventListener('input', () => {
  if (document.getElementById('field-password').classList.contains('has-error')) {
    setError('field-password', passwordEl.value === '');
  }
});

function showMsg(id, text, ok) {
  const el = document.getElementById(id);
  el.textContent = text || '';
  el.classList.toggle('is-error', !ok);
  el.classList.toggle('is-ok', !!ok && !!text);
}

form.addEventListener('submit', async function (e) {
  e.preventDefault();
  let valid = true;

  if (!validateEmail(emailEl.value)) {
    setError('field-email', true);
    valid = false;
  } else {
    setError('field-email', false);
  }

  if (passwordEl.value === '') {
    setError('field-password', true);
    valid = false;
  } else {
    setError('field-password', false);
  }

  if (!valid) return;

  const btn = document.getElementById('btnLogin');
  btn.disabled = true;
  showMsg('loginMsg', 'Authenticating…', true);
  try {
    const resp = await API.apiFetch('/auth/login', {
      method: 'POST',
      body: { email: emailEl.value.trim(), password: passwordEl.value },
    });
    API.setToken(resp.access_token);
    API.setUser(resp.user);
    window.location.href = '../appcatalog/';
  } catch (err) {
    showMsg('loginMsg', err.message, false);
    btn.disabled = false;
  }
});

/* ── Show / hide password (login) ── */
document.getElementById('togglePw').addEventListener('click', function () {
  const isPassword = passwordEl.type === 'password';
  passwordEl.type = isPassword ? 'text' : 'password';
  this.querySelector('.icon-show').style.display = isPassword ? 'none'  : 'block';
  this.querySelector('.icon-hide').style.display = isPassword ? 'block' : 'none';
});

/* ── Register / Back navigation ── */
const card = document.querySelector('.card');

document.getElementById('btnRegister').addEventListener('click', function () {
  card.classList.add('show-register');
});

document.getElementById('btnBack').addEventListener('click', function () {
  card.classList.remove('show-register');
  document.getElementById('registerForm').reset();
  ['reg-firstname','reg-lastname','reg-dob','reg-email','reg-password','reg-confirm'].forEach(function (id) {
    setError('field-' + id, false);
  });
});

/* ── Show / hide password (register) ── */
function makeToggle(btnId, inputId) {
  document.getElementById(btnId).addEventListener('click', function () {
    const input = document.getElementById(inputId);
    const isPassword = input.type === 'password';
    input.type = isPassword ? 'text' : 'password';
    this.querySelector('.icon-show').style.display = isPassword ? 'none'  : 'block';
    this.querySelector('.icon-hide').style.display = isPassword ? 'block' : 'none';
  });
}
makeToggle('toggleRegPw',      'reg-password');
makeToggle('toggleRegConfirm', 'reg-confirm');

/* ── Register form validation ── */
const regForm    = document.getElementById('registerForm');
const regPwEl    = document.getElementById('reg-password');
const regConfEl  = document.getElementById('reg-confirm');

regForm.addEventListener('submit', async function (e) {
  e.preventDefault();
  let valid = true;

  function req(inputId, fieldId) {
    const empty = document.getElementById(inputId).value.trim() === '';
    setError(fieldId, empty);
    if (empty) valid = false;
  }

  req('reg-firstname', 'field-reg-firstname');
  req('reg-lastname',  'field-reg-lastname');
  req('reg-dob',       'field-reg-dob');

  const regEmail = document.getElementById('reg-email');
  const emailOk = validateEmail(regEmail.value);
  setError('field-reg-email', !emailOk);
  if (!emailOk) valid = false;

  const pwOk = regPwEl.value.length >= 8;
  setError('field-reg-password', !pwOk);
  if (!pwOk) valid = false;

  const confirmOk = regConfEl.value === regPwEl.value && regConfEl.value !== '';
  setError('field-reg-confirm', !confirmOk);
  if (!confirmOk) valid = false;

  if (!valid) return;

  // The backend register endpoint only accepts email + password (there is no
  // profile endpoint yet), so the name/DOB fields are collected but not sent.
  const submitBtn = regForm.querySelector('button[type="submit"]');
  submitBtn.disabled = true;
  showMsg('registerMsg', 'Creating account…', true);
  try {
    await API.apiFetch('/auth/register', {
      method: 'POST',
      body: { email: regEmail.value.trim(), password: regPwEl.value },
    });
    showMsg('registerMsg', 'Account created — log in to continue.', true);
    // Pre-fill the login email and slide back to the login form.
    emailEl.value = regEmail.value.trim();
    setTimeout(function () {
      card.classList.remove('show-register');
      showMsg('loginMsg', 'Account created — please log in.', true);
    }, 900);
  } catch (err) {
    showMsg('registerMsg', err.message, false);
  } finally {
    submitBtn.disabled = false;
  }
});

/* ── Live validation on register fields ── */
regPwEl.addEventListener('input', function () {
  if (document.getElementById('field-reg-password').classList.contains('has-error')) {
    setError('field-reg-password', regPwEl.value.length < 8);
  }
});

regConfEl.addEventListener('input', function () {
  if (document.getElementById('field-reg-confirm').classList.contains('has-error')) {
    setError('field-reg-confirm', regConfEl.value !== regPwEl.value);
  }
});

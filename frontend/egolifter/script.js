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

/* ── Auth guard + operator identity + header actions ── */
API.requireAuth();
(function () {
  const user = API.getUser();
  const op = document.getElementById('operator');
  if (op && user && user.email) op.textContent = user.email;
  document.getElementById('btnCatalog').addEventListener('click', () => {
    window.location.href = '../appcatalog/';
  });
  document.getElementById('btnLogout').addEventListener('click', () => API.logout());
})();

/* ── Global status banner ── */
let statusTimer = null;
function showStatus(message, ok) {
  const el = document.getElementById('statusBanner');
  el.textContent = message;
  el.classList.remove('is-error', 'is-ok', 'show');
  el.classList.add(ok ? 'is-ok' : 'is-error', 'show');
  if (statusTimer) clearTimeout(statusTimer);
  statusTimer = setTimeout(() => el.classList.remove('show'), 4000);
}

/* ── Sidebar nav active state ── */
const buttons = document.querySelectorAll('.nav-btn');

buttons.forEach(btn => {
  btn.addEventListener('click', () => {
    buttons.forEach(b => b.classList.remove('active'));
    btn.classList.add('active');
    renderSection(btn.dataset.section);
  });
});

/* ── Section renderer ── */
function renderSection(section) {
  const main = document.querySelector('.main-content');
  if (section === 'nutrition') renderNutrition(main);
  else if (section === 'training') renderTraining(main);
  else if (section === 'recipe') renderRecipe(main);
  else if (section === 'analytics') renderAnalytics(main);
  else if (section === 'egolifter') renderEgolifter(main);
  else if (section === 'settings') renderSettings(main);
  else main.innerHTML = '';
}

/* ── Nutrition food log ── */
function createFoodEntry(foods) {
  const entry = document.createElement('div');
  entry.className = 'food-entry';

  // Optional picker: choose a saved food to auto-fill the editable fields below.
  if (foods && foods.length) {
    const pickWrap = document.createElement('div');
    pickWrap.className = 'field food-pick';
    pickWrap.innerHTML = `
      <label>Pick from my foods (optional)</label>
      <select data-field="food_ref">
        <option value="">— type manually —</option>
        ${foods.map(f => `<option value="${f.id}">${f.name}</option>`).join('')}
      </select>
    `;
    entry.appendChild(pickWrap);
  }

  const fields = [
    { id: 'name',     label: 'Food Name',   type: 'text',   placeholder: 'e.g. Chicken Breast' },
    { id: 'weight',   label: 'Weight (g)',   type: 'number', placeholder: '0' },
    { id: 'calories', label: 'Calories',     type: 'number', placeholder: '0' },
    { id: 'protein',  label: 'Protein (g)',  type: 'number', placeholder: '0' },
    { id: 'carbs',    label: 'Carbs (g)',    type: 'number', placeholder: '0' },
    { id: 'fats',     label: 'Fats (g)',     type: 'number', placeholder: '0' },
  ];

  fields.forEach(f => {
    const wrap = document.createElement('div');
    wrap.className = 'field';
    wrap.innerHTML = `
      <label>${f.label}</label>
      <input type="${f.type}" data-field="${f.id}" placeholder="${f.placeholder}" ${f.type === 'number' ? 'min="0" step="0.1"' : ''} />
    `;
    entry.appendChild(wrap);
  });

  // Wire the picker: selecting a food fills name + macros (per-100g × weight); the
  // fields stay editable and recompute when the weight changes for the picked food.
  const picker = entry.querySelector('[data-field="food_ref"]');
  if (picker) {
    const round1 = n => Math.round(n * 10) / 10;
    const fillMacros = food => {
      const weight = +entry.querySelector('[data-field="weight"]').value || 0;
      const factor = weight / 100;
      entry.querySelector('[data-field="calories"]').value = round1(food.calories_100 * factor);
      entry.querySelector('[data-field="protein"]').value  = round1(food.protein_100 * factor);
      entry.querySelector('[data-field="carbs"]').value    = round1(food.carbohydrates_100 * factor);
      entry.querySelector('[data-field="fats"]').value     = round1(food.fat_100 * factor);
    };
    picker.addEventListener('change', () => {
      const food = foods.find(f => String(f.id) === picker.value);
      if (!food) return;
      entry.querySelector('[data-field="name"]').value = food.name;
      fillMacros(food);
    });
    entry.querySelector('[data-field="weight"]').addEventListener('input', () => {
      const food = foods.find(f => String(f.id) === picker.value);
      if (food) fillMacros(food);
    });
  }

  const removeBtn = document.createElement('button');
  removeBtn.className = 'remove-btn';
  removeBtn.type = 'button';
  removeBtn.textContent = '×';
  removeBtn.addEventListener('click', () => {
    const list = document.querySelector('.food-entries');
    if (list.children.length > 1) {
      entry.remove();
      updateRemoveButtons();
    }
  });
  entry.appendChild(removeBtn);

  return entry;
}

function updateRemoveButtons() {
  const list = document.querySelector('.food-entries');
  const btns = list.querySelectorAll('.remove-btn');
  btns.forEach(btn => {
    btn.style.visibility = list.children.length > 1 ? 'visible' : 'hidden';
  });
}

function renderLogFood(container) {
  container.innerHTML = '<div class="list-loading">// LOADING…</div>';

  // Load the saved foods catalog so each entry can offer a picker; fall back to an
  // empty list (manual-only entries) if the fetch fails so logging still works.
  API.apiFetch('/food/view')
    .then(foods => buildLogFoodForm(container, foods || []))
    .catch(() => buildLogFoodForm(container, []));
}

function buildLogFoodForm(container, foods) {
  container.innerHTML = '';

  const form = document.createElement('form');
  form.className = 'log-food-form';
  form.innerHTML = `
    <div class="training-name-wrap">
      <label>Meal Name</label>
      <input type="text" data-field="mealName" placeholder="e.g. Breakfast" />
    </div>
    <div class="food-entries"></div>
    <div class="nutrition-actions">
      <button class="add-food-btn" type="button">+ ADD FOOD</button>
      <button class="log-meal-btn" type="submit">LOG MEAL</button>
    </div>
  `;

  container.appendChild(form);

  const list = form.querySelector('.food-entries');
  list.appendChild(createFoodEntry(foods));
  updateRemoveButtons();

  form.querySelector('.add-food-btn').addEventListener('click', () => {
    list.appendChild(createFoodEntry(foods));
    updateRemoveButtons();
  });

  form.addEventListener('submit', async e => {
    e.preventDefault();
    const body = {
      name: form.querySelector('[data-field="mealName"]').value.trim(),
      foods: [...list.querySelectorAll('.food-entry')].map(entry => ({
        name:          entry.querySelector('[data-field="name"]').value.trim(),
        weight_g:      +entry.querySelector('[data-field="weight"]').value,
        calories:      +entry.querySelector('[data-field="calories"]').value,
        protein:       +entry.querySelector('[data-field="protein"]').value,
        carbohydrates: +entry.querySelector('[data-field="carbs"]').value,
        fat:           +entry.querySelector('[data-field="fats"]').value,
      })),
    };
    const btn = form.querySelector('.log-meal-btn');
    btn.disabled = true;
    try {
      const meal = await API.apiFetch('/meal/create', { method: 'POST', body });
      showStatus('Meal "' + meal.name + '" logged (' + Math.round(meal.total_calories) + ' kcal).', true);
      renderLogFood(container);
    } catch (err) {
      showStatus(err.message, false);
    } finally {
      btn.disabled = false;
    }
  });
}

function renderCreateFood(container) {
  container.innerHTML = '';

  const form = document.createElement('form');
  form.className = 'create-food-form';

  const fields = [
    { id: 'name',     label: 'Food Name',   type: 'text',   placeholder: 'e.g. Chicken Breast', flex: 2 },
    { id: 'calories', label: 'Calories',     type: 'number', placeholder: '0',                   flex: 1 },
    { id: 'protein',  label: 'Protein (g)',  type: 'number', placeholder: '0',                   flex: 1 },
    { id: 'carbs',    label: 'Carbs (g)',    type: 'number', placeholder: '0',                   flex: 1 },
    { id: 'fats',     label: 'Fats (g)',     type: 'number', placeholder: '0',                   flex: 1 },
  ];

  const fieldsHTML = fields.map(f => `
    <div class="field" style="flex:${f.flex}">
      <label>${f.label}</label>
      <input type="${f.type}" data-field="${f.id}" placeholder="${f.placeholder}" ${f.type === 'number' ? 'min="0"' : ''} />
    </div>
  `).join('');

  form.innerHTML = `
    <div class="per-100g-badge">[ VALUES PER 100G ]</div>
    <div class="create-fields">${fieldsHTML}</div>
    <div class="nutrition-actions">
      <button class="log-meal-btn" type="submit">CREATE FOOD</button>
    </div>
  `;

  container.appendChild(form);

  form.addEventListener('submit', async e => {
    e.preventDefault();
    const body = {
      name:              form.querySelector('[data-field="name"]').value.trim(),
      calories_100:      +form.querySelector('[data-field="calories"]').value,
      protein_100:       +form.querySelector('[data-field="protein"]').value,
      carbohydrates_100: +form.querySelector('[data-field="carbs"]').value,
      fat_100:           +form.querySelector('[data-field="fats"]').value,
    };
    const btn = form.querySelector('.log-meal-btn');
    btn.disabled = true;
    try {
      const food = await API.apiFetch('/food/create', { method: 'POST', body });
      showStatus('Food "' + food.name + '" saved to your catalog.', true);
      renderCreateFood(container);
    } catch (err) {
      showStatus(err.message, false);
    } finally {
      btn.disabled = false;
    }
  });
}

function renderMyFoods(container) {
  container.innerHTML = '<div class="list-loading">// LOADING…</div>';
  API.apiFetch('/food/view').then(foods => {
    container.innerHTML = '';
    if (!foods || foods.length === 0) {
      container.innerHTML = '<div class="view-recipes-empty"><span>// NO FOODS SAVED</span></div>';
      return;
    }
    foods.forEach(food => {
      const row = document.createElement('div');
      row.className = 'data-row';
      row.innerHTML = `
        <div class="data-main">${food.name}</div>
        <div class="data-macros">${food.calories_100} kcal · P ${food.protein_100} · C ${food.carbohydrates_100} · F ${food.fat_100} <span class="per100">/100g</span></div>
        <button class="remove-btn" type="button" title="Delete">×</button>
      `;
      row.querySelector('.remove-btn').addEventListener('click', async () => {
        try {
          await API.apiFetch('/food/delete?id=' + encodeURIComponent(food.id), { method: 'DELETE' });
          showStatus('Food "' + food.name + '" deleted.', true);
          renderMyFoods(container);
        } catch (err) {
          showStatus(err.message, false);
        }
      });
      container.appendChild(row);
    });
  }).catch(err => {
    container.innerHTML = '';
    showStatus(err.message, false);
  });
}

// mealTimeStr formats a meal's created_at as e.g. "Jun 12 · 17:40" in local time.
function mealTimeStr(createdAt) {
  const eaten = new Date(createdAt);
  return `${eaten.toLocaleDateString([], { month: 'short', day: 'numeric' })} · ${eaten.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })}`;
}

// openMealModal shows a popup (~75% of the screen, page behind blurred) listing
// every food eaten in the given meal with its macros.
function openMealModal(meal) {
  const backdrop = document.createElement('div');
  backdrop.className = 'modal-backdrop';
  backdrop.innerHTML = `
    <div class="meal-modal">
      <div class="meal-modal-head">
        <h2 class="panel-title"></h2>
        <span class="meal-time">${mealTimeStr(meal.created_at)}</span>
        <button class="modal-close" type="button">×</button>
      </div>
      <div class="meal-modal-body"><div class="list-loading">// LOADING…</div></div>
    </div>
  `;
  backdrop.querySelector('.panel-title').textContent = `// ${meal.name}`;
  document.body.appendChild(backdrop);

  function close() {
    document.removeEventListener('keydown', onKeydown);
    backdrop.remove();
  }
  function onKeydown(e) {
    if (e.key === 'Escape') close();
  }
  document.addEventListener('keydown', onKeydown);
  backdrop.querySelector('.modal-close').addEventListener('click', close);
  backdrop.addEventListener('click', e => {
    if (e.target === backdrop) close();
  });

  const body = backdrop.querySelector('.meal-modal-body');
  API.apiFetch('/meal/view?id=' + encodeURIComponent(meal.id)).then(full => {
    body.innerHTML = '';
    if (!full || !full.foods || full.foods.length === 0) {
      body.innerHTML = '<div class="view-recipes-empty"><span>// NO FOODS IN THIS MEAL</span></div>';
      return;
    }
    full.foods.forEach(food => {
      const row = document.createElement('div');
      row.className = 'data-row';
      row.innerHTML = `
        <div class="data-main">${food.food_name} <span class="per100">· ${Math.round(food.weight_g)} g</span></div>
        <div class="data-macros">${Math.round(food.total_calories)} kcal · P ${Math.round(food.total_protein)} · C ${Math.round(food.total_carbohydrates)} · F ${Math.round(food.total_fat)}</div>
      `;
      body.appendChild(row);
    });
    const totals = document.createElement('div');
    totals.className = 'meal-modal-totals';
    totals.textContent = `// TOTAL: ${Math.round(full.total_calories)} KCAL · P ${Math.round(full.total_protein)} · C ${Math.round(full.total_carbohydrates)} · F ${Math.round(full.total_fat)}`;
    body.appendChild(totals);
  }).catch(err => {
    close();
    showStatus(err.message, false);
  });
}

function renderMyMeals(container) {
  // Format as local YYYY-MM-DD (toISOString would shift the date across timezones).
  const fmtDate = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const today = fmtDate(new Date());

  container.innerHTML = `
    <div class="meal-filter-bar">
      <div class="meal-quick-picks">
        <button class="quick-btn active" type="button" data-range="today">TODAY</button>
        <button class="quick-btn" type="button" data-range="week">THIS WEEK</button>
        <button class="quick-btn" type="button" data-range="month">THIS MONTH</button>
        <button class="quick-btn" type="button" data-range="year">THIS YEAR</button>
      </div>
      <div class="meal-date-fields">
        <div class="field">
          <label>From</label>
          <input type="date" class="meal-date-from">
        </div>
        <div class="field">
          <label>To</label>
          <input type="date" class="meal-date-to">
        </div>
      </div>
    </div>
    <div class="calories-today">// TODAY: — KCAL</div>
    <div class="meal-list"></div>
  `;

  const fromInput = container.querySelector('.meal-date-from');
  const toInput = container.querySelector('.meal-date-to');
  const listEl = container.querySelector('.meal-list');

  function loadMeals() {
    listEl.innerHTML = '<div class="list-loading">// LOADING…</div>';
    API.apiFetch('/meal/by-date?date_from=' + encodeURIComponent(fromInput.value) + '&date_to=' + encodeURIComponent(toInput.value)).then(meals => {
      listEl.innerHTML = '';
      if (!meals || meals.length === 0) {
        listEl.innerHTML = '<div class="view-recipes-empty"><span>// NO MEALS LOGGED</span></div>';
        return;
      }
      meals.forEach(meal => {
        const row = document.createElement('div');
        row.className = 'data-row';
        row.innerHTML = `
          <div class="data-main">${meal.name}</div>
          <div class="data-macros">${Math.round(meal.total_calories)} kcal · P ${Math.round(meal.total_protein)} · C ${Math.round(meal.total_carbohydrates)} · F ${Math.round(meal.total_fat)}</div>
          <span class="meal-time">${mealTimeStr(meal.created_at)}</span>
          <button class="view-btn" type="button">VIEW</button>
          <button class="remove-btn" type="button" title="Delete">×</button>
        `;
        row.querySelector('.view-btn').addEventListener('click', () => openMealModal(meal));
        row.querySelector('.remove-btn').addEventListener('click', async () => {
          try {
            await API.apiFetch('/meal/del?id=' + encodeURIComponent(meal.id), { method: 'DELETE' });
            showStatus('Meal "' + meal.name + '" deleted.', true);
            loadMeals();
            loadTodayCalories();
          } catch (err) {
            showStatus(err.message, false);
          }
        });
        listEl.appendChild(row);
      });
    }).catch(err => {
      listEl.innerHTML = '';
      showStatus(err.message, false);
    });
  }

  function loadTodayCalories() {
    const el = container.querySelector('.calories-today');
    API.apiFetch('/meal/by-date?date_from=' + today + '&date_to=' + today).then(meals => {
      const total = (meals || []).reduce((sum, m) => sum + m.total_calories, 0);
      el.textContent = `// TODAY: ${Math.round(total)} KCAL`;
    }).catch(() => {
      el.textContent = '// TODAY: — KCAL';
    });
  }

  // Calendar periods ending today; the week starts on Monday.
  const ranges = {
    today: () => { const d = new Date(); return [d, d]; },
    week: () => {
      const d = new Date();
      const start = new Date(d);
      start.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return [start, d];
    },
    month: () => { const d = new Date(); return [new Date(d.getFullYear(), d.getMonth(), 1), d]; },
    year: () => { const d = new Date(); return [new Date(d.getFullYear(), 0, 1), d]; },
  };

  container.querySelectorAll('.quick-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      container.querySelectorAll('.quick-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      const [from, to] = ranges[btn.dataset.range]();
      fromInput.value = fmtDate(from);
      toInput.value = fmtDate(to);
      loadMeals();
    });
  });

  [fromInput, toInput].forEach(input => {
    input.addEventListener('change', () => {
      container.querySelectorAll('.quick-btn').forEach(b => b.classList.remove('active'));
      loadMeals();
    });
  });

  fromInput.value = today;
  toInput.value = today;
  loadMeals();
  loadTodayCalories();
}

function renderNutrition(container) {
  container.innerHTML = '';

  const panel = document.createElement('div');
  panel.className = 'nutrition-panel';

  panel.innerHTML = `
    <h2 class="panel-title">// NUTRITION</h2>
    <div class="nutrition-tabs">
      <button class="tab-btn active" type="button" data-tab="log">LOG MEAL</button>
      <button class="tab-btn" type="button" data-tab="create">CREATE FOOD</button>
      <button class="tab-btn" type="button" data-tab="foods">MY FOODS</button>
      <button class="tab-btn" type="button" data-tab="meals">MY MEALS</button>
    </div>
    <div class="tab-content"></div>
  `;

  container.appendChild(panel);

  const tabContent = panel.querySelector('.tab-content');
  const renderers = {
    log: renderLogFood,
    create: renderCreateFood,
    foods: renderMyFoods,
    meals: renderMyMeals,
  };

  panel.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      panel.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      renderers[btn.dataset.tab](tabContent);
    });
  });

  renderLogFood(tabContent);
}

/* ── Training log ── */
function createExerciseEntry(listEl) {
  const entry = document.createElement('div');
  entry.className = 'exercise-entry';

  const fields = [
    { id: 'name',   label: 'Exercise Name', type: 'text',   placeholder: 'e.g. Squat',  flex: 2 },
    { id: 'sets',   label: 'Sets',          type: 'number', placeholder: '0',            flex: 1 },
    { id: 'weight', label: 'Weight (kg)',   type: 'number', placeholder: '0',            flex: 1, step: '0.1' },
    { id: 'reps',   label: 'Reps',          type: 'number', placeholder: '0',            flex: 1 },
    { id: 'note',   label: 'Note',          type: 'text',   placeholder: 'optional…',   flex: 2 },
  ];

  fields.forEach(f => {
    const wrap = document.createElement('div');
    wrap.className = 'field';
    wrap.style.flex = f.flex;
    wrap.innerHTML = `
      <label>${f.label}</label>
      <input type="${f.type}" data-field="${f.id}" placeholder="${f.placeholder}" ${f.type === 'number' ? `min="0" step="${f.step || '1'}"` : ''} />
    `;
    entry.appendChild(wrap);
  });

  const removeBtn = document.createElement('button');
  removeBtn.className = 'remove-btn';
  removeBtn.type = 'button';
  removeBtn.textContent = '×';
  removeBtn.addEventListener('click', () => {
    if (listEl.children.length > 1) {
      entry.remove();
      updateExerciseRemoveButtons(listEl);
    }
  });
  entry.appendChild(removeBtn);

  return entry;
}

function updateExerciseRemoveButtons(listEl) {
  listEl.querySelectorAll('.remove-btn').forEach(btn => {
    btn.style.visibility = listEl.children.length > 1 ? 'visible' : 'hidden';
  });
}

// Log Training records a performed workout. Picking a routine pre-fills
// editable exercise rows (the routine is just a time-saver) — tweak weights,
// add or remove exercises, then submit (backend: POST /training/log
// {routine_id, name, exercises}).
function renderLogTraining(container) {
  container.innerHTML = '';

  const form = document.createElement('form');
  form.className = 'log-training-form';
  form.innerHTML = `
    <div class="training-name-wrap">
      <label>Pick a Routine to Log</label>
      <select data-field="routine"><option value="">// loading routines…</option></select>
    </div>
    <div class="exercise-entries"></div>
    <div class="nutrition-actions">
      <button class="add-food-btn" type="button">+ ADD EXERCISE</button>
      <button class="log-meal-btn" type="submit">LOG WORKOUT</button>
    </div>
  `;

  container.appendChild(form);

  const select = form.querySelector('[data-field="routine"]');
  const list = form.querySelector('.exercise-entries');
  let routines = [];

  // Pre-fill the editable rows from the picked routine's entries.
  function fillFromRoutine(routine) {
    list.innerHTML = '';
    (routine.entries || []).forEach(entry => {
      const row = createExerciseEntry(list);
      row.querySelector('[data-field="name"]').value = entry.name;
      row.querySelector('[data-field="weight"]').value = entry.weight_kg;
      row.querySelector('[data-field="reps"]').value = entry.reps;
      list.appendChild(row);
    });
    if (!list.children.length) list.appendChild(createExerciseEntry(list));
    updateExerciseRemoveButtons(list);
  }

  // Populate the routine picker.
  API.apiFetch('/training/routine/view').then(loaded => {
    routines = loaded || [];
    if (routines.length === 0) {
      select.innerHTML = '<option value="">No routines — create one first</option>';
      return;
    }
    select.innerHTML = '<option value="">— pick a routine —</option>' + routines
      .map(r => `<option value="${r.id}">${r.name} (${(r.entries || []).length} exercises)</option>`)
      .join('');
  }).catch(err => {
    select.innerHTML = '<option value="">Failed to load routines</option>';
    showStatus(err.message, false);
  });

  select.addEventListener('change', () => {
    const routine = routines.find(r => String(r.id) === select.value);
    if (routine) fillFromRoutine(routine);
  });

  form.querySelector('.add-food-btn').addEventListener('click', () => {
    list.appendChild(createExerciseEntry(list));
    updateExerciseRemoveButtons(list);
  });

  form.addEventListener('submit', async e => {
    e.preventDefault();
    const routineID = select.value;
    if (!routineID) {
      showStatus('Pick a routine to log (create one in the Create Training tab first).', false);
      return;
    }
    const routine = routines.find(r => String(r.id) === routineID);
    // Exercise entries only persist name + weight_kg + reps (sets/note are UI-only).
    const body = {
      routine_id: routineID,
      name: routine ? routine.name : '',
      exercises: [...list.querySelectorAll('.exercise-entry')].map(entry => ({
        name:      entry.querySelector('[data-field="name"]').value.trim(),
        weight_kg: +entry.querySelector('[data-field="weight"]').value,
        reps:      +entry.querySelector('[data-field="reps"]').value,
      })),
    };
    const btn = form.querySelector('.log-meal-btn');
    btn.disabled = true;
    try {
      const workout = await API.apiFetch('/training/log', { method: 'POST', body });
      showStatus('Workout "' + workout.name + '" logged.', true);
      renderLogTraining(container);
    } catch (err) {
      showStatus(err.message, false);
    } finally {
      btn.disabled = false;
    }
  });
}

function renderCreateTraining(container) {
  container.innerHTML = '';

  const form = document.createElement('form');
  form.className = 'create-training-form';
  form.innerHTML = `
    <div class="training-name-wrap">
      <label>Training Name</label>
      <input type="text" data-field="trainingName" placeholder="e.g. Lower Body Day" />
    </div>
    <div class="exercise-entries"></div>
    <div class="nutrition-actions">
      <button class="add-food-btn" type="button">+ ADD EXERCISE</button>
      <button class="log-meal-btn" type="submit">CREATE TRAINING</button>
    </div>
  `;

  container.appendChild(form);

  const list = form.querySelector('.exercise-entries');
  list.appendChild(createExerciseEntry(list));
  updateExerciseRemoveButtons(list);

  form.querySelector('.add-food-btn').addEventListener('click', () => {
    list.appendChild(createExerciseEntry(list));
    updateExerciseRemoveButtons(list);
  });

  form.addEventListener('submit', async e => {
    e.preventDefault();
    // Backend routine entries only carry name + weight_kg + reps (sets/note are
    // collected for the user's convenience but not persisted).
    const body = {
      name: form.querySelector('[data-field="trainingName"]').value.trim(),
      entries: [...list.querySelectorAll('.exercise-entry')].map(entry => ({
        name:      entry.querySelector('[data-field="name"]').value.trim(),
        weight_kg: +entry.querySelector('[data-field="weight"]').value,
        reps:      +entry.querySelector('[data-field="reps"]').value,
      })),
    };
    const btn = form.querySelector('.log-meal-btn');
    btn.disabled = true;
    try {
      const routine = await API.apiFetch('/training/routine/create', { method: 'POST', body });
      showStatus('Routine "' + routine.name + '" created.', true);
      renderCreateTraining(container);
    } catch (err) {
      showStatus(err.message, false);
    } finally {
      btn.disabled = false;
    }
  });
}

// openWorkoutModal shows a popup (~75% of the screen, page behind blurred)
// listing every exercise performed in the given workout.
function openWorkoutModal(workout) {
  const backdrop = document.createElement('div');
  backdrop.className = 'modal-backdrop';
  backdrop.innerHTML = `
    <div class="meal-modal">
      <div class="meal-modal-head">
        <h2 class="panel-title"></h2>
        <span class="meal-time">${mealTimeStr(workout.performed_at)}</span>
        <button class="modal-close" type="button">×</button>
      </div>
      <div class="meal-modal-body"></div>
    </div>
  `;
  backdrop.querySelector('.panel-title').textContent = `// ${workout.name}`;
  document.body.appendChild(backdrop);

  function close() {
    document.removeEventListener('keydown', onKeydown);
    backdrop.remove();
  }
  function onKeydown(e) {
    if (e.key === 'Escape') close();
  }
  document.addEventListener('keydown', onKeydown);
  backdrop.querySelector('.modal-close').addEventListener('click', close);
  backdrop.addEventListener('click', e => {
    if (e.target === backdrop) close();
  });

  const body = backdrop.querySelector('.meal-modal-body');
  const exercises = workout.exercises || [];
  if (exercises.length === 0) {
    body.innerHTML = '<div class="view-recipes-empty"><span>// NO EXERCISES IN THIS WORKOUT</span></div>';
    return;
  }
  exercises.forEach(ex => {
    const row = document.createElement('div');
    row.className = 'data-row';
    row.innerHTML = `
      <div class="data-main">${ex.name}</div>
      <div class="data-macros">${ex.weight_kg} kg × ${ex.reps} reps</div>
    `;
    body.appendChild(row);
  });
}

// My Trainings: logged workouts filtered by date range, with quick-picks.
function renderMyTrainings(container) {
  const fmtDate = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  const today = fmtDate(new Date());

  container.innerHTML = `
    <div class="meal-filter-bar">
      <div class="meal-quick-picks">
        <button class="quick-btn active" type="button" data-range="today">TODAY</button>
        <button class="quick-btn" type="button" data-range="week">THIS WEEK</button>
        <button class="quick-btn" type="button" data-range="month">THIS MONTH</button>
        <button class="quick-btn" type="button" data-range="year">THIS YEAR</button>
      </div>
      <div class="meal-date-fields">
        <div class="field">
          <label>From</label>
          <input type="date" class="meal-date-from">
        </div>
        <div class="field">
          <label>To</label>
          <input type="date" class="meal-date-to">
        </div>
      </div>
    </div>
    <div class="meal-list"></div>
  `;

  const fromInput = container.querySelector('.meal-date-from');
  const toInput = container.querySelector('.meal-date-to');
  const listEl = container.querySelector('.meal-list');

  function loadWorkouts() {
    listEl.innerHTML = '<div class="list-loading">// LOADING…</div>';
    API.apiFetch('/training/by-date?date_from=' + encodeURIComponent(fromInput.value) + '&date_to=' + encodeURIComponent(toInput.value)).then(workouts => {
      listEl.innerHTML = '';
      if (!workouts || workouts.length === 0) {
        listEl.innerHTML = '<div class="view-recipes-empty"><span>// NO WORKOUTS LOGGED</span></div>';
        return;
      }
      workouts.forEach(workout => {
        const row = document.createElement('div');
        row.className = 'data-row';
        row.innerHTML = `
          <div class="data-main">${workout.name}</div>
          <div class="data-macros">${(workout.exercises || []).length} exercises</div>
          <span class="meal-time">${mealTimeStr(workout.performed_at)}</span>
          <button class="view-btn" type="button">VIEW</button>
          <button class="remove-btn" type="button" title="Delete">×</button>
        `;
        row.querySelector('.view-btn').addEventListener('click', () => openWorkoutModal(workout));
        row.querySelector('.remove-btn').addEventListener('click', async () => {
          try {
            await API.apiFetch('/training/del?id=' + encodeURIComponent(workout.id), { method: 'DELETE' });
            showStatus('Workout "' + workout.name + '" deleted.', true);
            loadWorkouts();
          } catch (err) {
            showStatus(err.message, false);
          }
        });
        listEl.appendChild(row);
      });
    }).catch(err => {
      listEl.innerHTML = '';
      showStatus(err.message, false);
    });
  }

  // Calendar periods ending today; the week starts on Monday.
  const ranges = {
    today: () => { const d = new Date(); return [d, d]; },
    week: () => {
      const d = new Date();
      const start = new Date(d);
      start.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return [start, d];
    },
    month: () => { const d = new Date(); return [new Date(d.getFullYear(), d.getMonth(), 1), d]; },
    year: () => { const d = new Date(); return [new Date(d.getFullYear(), 0, 1), d]; },
  };

  container.querySelectorAll('.quick-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      container.querySelectorAll('.quick-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      const [from, to] = ranges[btn.dataset.range]();
      fromInput.value = fmtDate(from);
      toInput.value = fmtDate(to);
      loadWorkouts();
    });
  });

  [fromInput, toInput].forEach(input => {
    input.addEventListener('change', () => {
      container.querySelectorAll('.quick-btn').forEach(b => b.classList.remove('active'));
      loadWorkouts();
    });
  });

  fromInput.value = today;
  toInput.value = today;
  loadWorkouts();
}

function renderTraining(container) {
  container.innerHTML = '';

  const panel = document.createElement('div');
  panel.className = 'training-panel';

  panel.innerHTML = `
    <h2 class="panel-title">// TRAINING</h2>
    <div class="nutrition-tabs">
      <button class="tab-btn active" type="button" data-tab="log">LOG TRAINING</button>
      <button class="tab-btn" type="button" data-tab="create">CREATE TRAINING</button>
      <button class="tab-btn" type="button" data-tab="view">MY TRAININGS</button>
    </div>
    <div class="tab-content"></div>
  `;

  container.appendChild(panel);

  const tabContent = panel.querySelector('.tab-content');
  const renderers = {
    log: renderLogTraining,
    create: renderCreateTraining,
    view: renderMyTrainings,
  };

  panel.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      panel.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      renderers[btn.dataset.tab](tabContent);
    });
  });

  renderLogTraining(tabContent);
}

/* ── Recipe ── */
// A recipe ingredient references a saved food (food_id) plus a weight in grams.
function createIngredientEntry(listEl, foods) {
  const entry = document.createElement('div');
  entry.className = 'ingredient-entry';

  const options = foods.map(f => `<option value="${f.id}">${f.name}</option>`).join('');

  const foodWrap = document.createElement('div');
  foodWrap.className = 'field';
  foodWrap.style.flex = 2;
  foodWrap.innerHTML = `
    <label>Ingredient (from your foods)</label>
    <select data-field="food_id">${options}</select>
  `;
  entry.appendChild(foodWrap);

  const weightWrap = document.createElement('div');
  weightWrap.className = 'field';
  weightWrap.style.flex = 1;
  weightWrap.innerHTML = `
    <label>Weight (g)</label>
    <input type="number" data-field="weight_g" placeholder="0" min="0" />
  `;
  entry.appendChild(weightWrap);

  const removeBtn = document.createElement('button');
  removeBtn.className = 'remove-btn';
  removeBtn.type = 'button';
  removeBtn.textContent = '×';
  removeBtn.addEventListener('click', () => {
    if (listEl.children.length > 1) {
      entry.remove();
      updateExerciseRemoveButtons(listEl);
    }
  });
  entry.appendChild(removeBtn);

  return entry;
}

function renderCreateRecipe(container) {
  container.innerHTML = '<div class="list-loading">// LOADING YOUR FOODS…</div>';

  // Ingredients reference saved foods, so load the catalog first.
  API.apiFetch('/food/view').then(foods => {
    container.innerHTML = '';

    if (!foods || foods.length === 0) {
      container.innerHTML = '<div class="view-recipes-empty"><span>// CREATE A FOOD FIRST — recipes are built from your saved foods</span></div>';
      return;
    }

    const form = document.createElement('form');
    form.className = 'create-recipe-form';
    form.innerHTML = `
      <div class="training-name-wrap">
        <label>Recipe Name</label>
        <input type="text" data-field="recipeName" placeholder="e.g. High Protein Oatmeal" />
      </div>
      <div class="ingredient-entries"></div>
      <div class="nutrition-actions">
        <button class="add-food-btn" type="button">+ ADD INGREDIENT</button>
        <button class="log-meal-btn" type="submit">CREATE RECIPE</button>
      </div>
      <div class="recipe-notes-wrap">
        <label>Notes / Instructions</label>
        <textarea data-field="notes" placeholder="Describe how to prepare this recipe…"></textarea>
      </div>
    `;

    container.appendChild(form);

    const list = form.querySelector('.ingredient-entries');
    list.appendChild(createIngredientEntry(list, foods));
    updateExerciseRemoveButtons(list);

    form.querySelector('.add-food-btn').addEventListener('click', () => {
      list.appendChild(createIngredientEntry(list, foods));
      updateExerciseRemoveButtons(list);
    });

    form.addEventListener('submit', async e => {
      e.preventDefault();
      const notes = form.querySelector('[data-field="notes"]').value.trim();
      const body = {
        name: form.querySelector('[data-field="recipeName"]').value.trim(),
        notes: notes || null,
        ingredients: [...list.querySelectorAll('.ingredient-entry')].map(entry => ({
          food_id:  entry.querySelector('[data-field="food_id"]').value,
          weight_g: +entry.querySelector('[data-field="weight_g"]').value,
        })),
      };
      const btn = form.querySelector('.log-meal-btn');
      btn.disabled = true;
      try {
        const recipe = await API.apiFetch('/recipe/create', { method: 'POST', body });
        showStatus('Recipe "' + recipe.name + '" created.', true);
        renderCreateRecipe(container);
      } catch (err) {
        showStatus(err.message, false);
      } finally {
        btn.disabled = false;
      }
    });
  }).catch(err => {
    container.innerHTML = '';
    showStatus(err.message, false);
  });
}

// openRecipeModal shows a popup (~75% of the screen, page behind blurred) listing
// every ingredient of the recipe with its macros, the recipe totals and notes.
// The foods come from /recipe/getfoods, which returns the macros already
// adjusted for each ingredient's weight.
function openRecipeModal(recipe) {
  const backdrop = document.createElement('div');
  backdrop.className = 'modal-backdrop';
  backdrop.innerHTML = `
    <div class="meal-modal">
      <div class="meal-modal-head">
        <h2 class="panel-title"></h2>
        <span class="meal-time">${mealTimeStr(recipe.created_at)}</span>
        <button class="modal-close" type="button">×</button>
      </div>
      <div class="meal-modal-body"><div class="list-loading">// LOADING…</div></div>
    </div>
  `;
  backdrop.querySelector('.panel-title').textContent = `// ${recipe.name}`;
  document.body.appendChild(backdrop);

  function close() {
    document.removeEventListener('keydown', onKeydown);
    backdrop.remove();
  }
  function onKeydown(e) {
    if (e.key === 'Escape') close();
  }
  document.addEventListener('keydown', onKeydown);
  backdrop.querySelector('.modal-close').addEventListener('click', close);
  backdrop.addEventListener('click', e => {
    if (e.target === backdrop) close();
  });

  const body = backdrop.querySelector('.meal-modal-body');
  API.apiFetch('/recipe/getfoods?id=' + encodeURIComponent(recipe.id)).then(foods => {
    body.innerHTML = '';
    if (!foods || foods.length === 0) {
      body.innerHTML = '<div class="view-recipes-empty"><span>// NO INGREDIENTS IN THIS RECIPE</span></div>';
    } else {
      const totals = { cal: 0, prot: 0, carb: 0, fat: 0 };
      foods.forEach(food => {
        totals.cal += food.total_calories;
        totals.prot += food.total_protein;
        totals.carb += food.total_carbohydrates;
        totals.fat += food.total_fat;

        const row = document.createElement('div');
        row.className = 'data-row';
        row.innerHTML = `
          <div class="data-main">${food.food_name} <span class="per100">· ${Math.round(food.weight_g)} g</span></div>
          <div class="data-macros">${Math.round(food.total_calories)} kcal · P ${Math.round(food.total_protein)} · C ${Math.round(food.total_carbohydrates)} · F ${Math.round(food.total_fat)}${food.notes ? ' <span class="per100"></span>' : ''}</div>
        `;
        if (food.notes) row.querySelector('.data-macros .per100').textContent = `· ${food.notes}`;
        body.appendChild(row);
      });
      const totalsEl = document.createElement('div');
      totalsEl.className = 'meal-modal-totals';
      totalsEl.textContent = `// TOTAL: ${Math.round(totals.cal)} KCAL · P ${Math.round(totals.prot)} · C ${Math.round(totals.carb)} · F ${Math.round(totals.fat)}`;
      body.appendChild(totalsEl);
    }

    if (recipe.notes) {
      const notes = document.createElement('div');
      notes.className = 'recipe-modal-notes';
      notes.innerHTML = '<div class="notes-label">// NOTES</div><p></p>';
      notes.querySelector('p').textContent = recipe.notes;
      body.appendChild(notes);
    }
  }).catch(err => {
    close();
    showStatus(err.message, false);
  });
}

function renderViewRecipes(container) {
  container.innerHTML = '<div class="list-loading">// LOADING…</div>';
  API.apiFetch('/recipe/view').then(recipes => {
    container.innerHTML = '';
    if (!recipes || recipes.length === 0) {
      container.innerHTML = '<div class="view-recipes-empty"><span>// NO RECIPES SAVED</span></div>';
      return;
    }
    recipes.forEach(recipe => {
      const ings = (recipe.ingredients || [])
        .map(i => `${i.food_name} (${i.weight_g}g)`)
        .join(', ');
      const row = document.createElement('div');
      row.className = 'data-row';
      row.innerHTML = `
        <div class="data-main">${recipe.name}</div>
        <div class="data-macros">${ings || 'no ingredients'}</div>
        <button class="view-btn" type="button">VIEW</button>
        <button class="remove-btn" type="button" title="Delete">×</button>
      `;
      row.querySelector('.view-btn').addEventListener('click', () => openRecipeModal(recipe));
      row.querySelector('.remove-btn').addEventListener('click', async () => {
        try {
          await API.apiFetch('/recipe/del?id=' + encodeURIComponent(recipe.id), { method: 'DELETE' });
          showStatus('Recipe "' + recipe.name + '" deleted.', true);
          renderViewRecipes(container);
        } catch (err) {
          showStatus(err.message, false);
        }
      });
      container.appendChild(row);
    });
  }).catch(err => {
    container.innerHTML = '';
    showStatus(err.message, false);
  });
}

function renderRecipe(container) {
  container.innerHTML = '';

  const panel = document.createElement('div');
  panel.className = 'recipe-panel';

  panel.innerHTML = `
    <h2 class="panel-title">// RECIPE</h2>
    <div class="nutrition-tabs">
      <button class="tab-btn active" type="button" data-tab="create">CREATE RECIPE</button>
      <button class="tab-btn" type="button" data-tab="view">VIEW RECIPES</button>
    </div>
    <div class="tab-content"></div>
  `;

  container.appendChild(panel);

  const tabContent = panel.querySelector('.tab-content');

  panel.querySelectorAll('.tab-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      panel.querySelectorAll('.tab-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      if (btn.dataset.tab === 'create') renderCreateRecipe(tabContent);
      else renderViewRecipes(tabContent);
    });
  });

  renderCreateRecipe(tabContent);
}

/* ── Analytics ── */
// statCard returns the markup for one stat block; unit is optional.
function statCard(label, value, unit) {
  return `
    <div class="stat-card">
      <div class="stat-label">${label}</div>
      <div class="stat-value">${value}${unit ? ` <span class="stat-unit">${unit}</span>` : ''}</div>
    </div>`;
}

// renderSummaryBlocks paints the /analytics/summary response into the results
// area: a range banner, nutrition averages (per logged day) + period totals,
// and training stat cards.
function renderSummaryBlocks(el, s) {
  const round1 = n => Math.round(n * 10) / 10;
  const n = s.nutrition;
  const t = s.training;

  el.innerHTML = `
    <div class="calories-today">// ${s.date_from} → ${s.date_to} · ${s.days} DAY${s.days === 1 ? '' : 'S'}</div>

    <div class="subhead">// NUTRITION</div>

    <div class="subhead">// DAILY AVERAGE · PER LOGGED DAY</div>
    <div class="analytics-grid">
      ${statCard('Calories', Math.round(n.daily_avg.calories), 'kcal')}
      ${statCard('Protein', round1(n.daily_avg.protein), 'g')}
      ${statCard('Carbs', round1(n.daily_avg.carbohydrates), 'g')}
      ${statCard('Fat', round1(n.daily_avg.fat), 'g')}
    </div>

    <div class="subhead">// PERIOD TOTALS</div>
    <div class="analytics-table">
      <div class="data-row"><div class="data-main">Total Calories</div><div class="data-macros">${Math.round(n.total_calories)} kcal</div></div>
      <div class="data-row"><div class="data-main">Total Protein</div><div class="data-macros">${Math.round(n.total_protein)} g</div></div>
      <div class="data-row"><div class="data-main">Total Carbs</div><div class="data-macros">${Math.round(n.total_carbohydrates)} g</div></div>
      <div class="data-row"><div class="data-main">Total Fat</div><div class="data-macros">${Math.round(n.total_fat)} g</div></div>
      <div class="data-row"><div class="data-main">Meals Logged</div><div class="data-macros">${n.meals_logged}</div></div>
      <div class="data-row"><div class="data-main">Days Logged</div><div class="data-macros">${n.days_logged} / ${s.days}</div></div>
    </div>

    <div class="subhead">// TRAINING</div>
    <div class="analytics-grid">
      ${statCard('Workouts', t.workouts)}
      ${statCard('Days Trained', `${t.days_trained} / ${s.days}`)}
      ${statCard('Total Sets', t.total_sets)}
      ${statCard('Total Reps', t.total_reps)}
      ${statCard('Total Volume', Math.round(t.total_volume_kg), 'kg')}
    </div>
  `;
}

// Analytics: a date-range summary of nutrition + training, with quick-picks.
function renderAnalytics(container) {
  container.innerHTML = '';

  const fmtDate = d => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

  const panel = document.createElement('div');
  panel.className = 'analytics-panel';
  panel.innerHTML = `
    <h2 class="panel-title">// ANALYTICS</h2>
    <div class="meal-filter-bar">
      <div class="meal-quick-picks">
        <button class="quick-btn" type="button" data-range="today">TODAY</button>
        <button class="quick-btn active" type="button" data-range="week">THIS WEEK</button>
        <button class="quick-btn" type="button" data-range="month">THIS MONTH</button>
        <button class="quick-btn" type="button" data-range="year">THIS YEAR</button>
      </div>
      <div class="meal-date-fields">
        <div class="field">
          <label>From</label>
          <input type="date" class="meal-date-from">
        </div>
        <div class="field">
          <label>To</label>
          <input type="date" class="meal-date-to">
        </div>
      </div>
    </div>
    <div class="analytics-results"></div>
  `;

  container.appendChild(panel);

  const fromInput = panel.querySelector('.meal-date-from');
  const toInput = panel.querySelector('.meal-date-to');
  const resultsEl = panel.querySelector('.analytics-results');

  function loadSummary() {
    resultsEl.innerHTML = '<div class="list-loading">// CRUNCHING NUMBERS…</div>';
    API.apiFetch('/analytics/summary?date_from=' + encodeURIComponent(fromInput.value) + '&date_to=' + encodeURIComponent(toInput.value))
      .then(summary => renderSummaryBlocks(resultsEl, summary))
      .catch(err => {
        resultsEl.innerHTML = '';
        showStatus(err.message, false);
      });
  }

  // Calendar periods ending today; the week starts on Monday.
  const ranges = {
    today: () => { const d = new Date(); return [d, d]; },
    week: () => {
      const d = new Date();
      const start = new Date(d);
      start.setDate(d.getDate() - ((d.getDay() + 6) % 7));
      return [start, d];
    },
    month: () => { const d = new Date(); return [new Date(d.getFullYear(), d.getMonth(), 1), d]; },
    year: () => { const d = new Date(); return [new Date(d.getFullYear(), 0, 1), d]; },
  };

  panel.querySelectorAll('.quick-btn').forEach(btn => {
    btn.addEventListener('click', () => {
      panel.querySelectorAll('.quick-btn').forEach(b => b.classList.remove('active'));
      btn.classList.add('active');
      const [from, to] = ranges[btn.dataset.range]();
      fromInput.value = fmtDate(from);
      toInput.value = fmtDate(to);
      loadSummary();
    });
  });

  [fromInput, toInput].forEach(input => {
    input.addEventListener('change', () => {
      panel.querySelectorAll('.quick-btn').forEach(b => b.classList.remove('active'));
      loadSummary();
    });
  });

  // Default to the current week (analytics is range-oriented).
  const [from, to] = ranges.week();
  fromInput.value = fmtDate(from);
  toInput.value = fmtDate(to);
  loadSummary();
}

/* ── EgoLifter VIP ── */
// EgoLifter: a premium / VIP showcase panel. Static for now — a glowing hero
// with the perks the membership unlocks.
// EgoLifter bot: a chat with the DeepSeek ReAct agent. The conversation lives
// in the center; the user's past chats sit in a narrower panel on the right.
// Talks to the JWT-protected /egolifter/* API via the shared API.apiFetch.
function renderEgolifter(container) {
  container.innerHTML = '';

  // Per-visit state. The SPA re-invokes renderEgolifter on each nav click, so a
  // fresh visit starts on a blank chat; loadChats() restores the history list.
  let messages = [];     // [{ role: 'user' | 'assistant', content }]
  let currentChatId = 0; // 0 = new chat; set from the POST /egolifter/chat reply
  let chats = [];        // [{ chatId, title, createdAt, updatedAt }]

  const panel = document.createElement('div');
  panel.className = 'egobot-panel';
  panel.innerHTML = `
    <section class="egobot-chat">
      <div class="egobot-bar">
        <h2 class="panel-title">// EGOLIFTER BOT</h2>
        <button class="egobot-newchat" type="button">+ NEW CHAT</button>
      </div>
      <div class="egobot-messages" id="egobotMessages"></div>
      <div class="egobot-composer">
        <textarea class="egobot-input" id="egobotInput" rows="1"
                  placeholder="Message EgoLifter…"></textarea>
        <button class="egobot-send" id="egobotSend" type="button">SEND</button>
      </div>
    </section>
    <aside class="egobot-history">
      <h3 class="egobot-history-title">HISTORY</h3>
      <div class="egobot-history-list" id="egobotHistoryList"></div>
    </aside>
  `;
  container.appendChild(panel);

  const messagesEl = panel.querySelector('#egobotMessages');
  const inputEl = panel.querySelector('#egobotInput');
  const sendBtn = panel.querySelector('#egobotSend');
  const newChatBtn = panel.querySelector('.egobot-newchat');
  const historyEl = panel.querySelector('#egobotHistoryList');

  /* ── Message rendering ── */
  // textContent (never innerHTML) keeps replies XSS-safe; CSS white-space:
  // pre-wrap preserves the agent's line breaks.
  function renderMessage(role, content) {
    const wrap = document.createElement('div');
    wrap.className = 'egobot-msg ' + role;
    if (role === 'assistant') {
      const label = document.createElement('div');
      label.className = 'egobot-msg-label';
      label.textContent = 'EGOLIFTER';
      wrap.appendChild(label);
      const body = document.createElement('div');
      body.className = 'egobot-msg-body';
      body.textContent = content;
      wrap.appendChild(body);
    } else {
      wrap.textContent = content;
    }
    messagesEl.appendChild(wrap);
  }

  function renderAll() {
    messagesEl.innerHTML = '';
    messages.forEach(m => renderMessage(m.role, m.content));
    scrollToBottom();
  }

  function scrollToBottom() {
    messagesEl.scrollTop = messagesEl.scrollHeight;
  }

  let typingEl = null;
  function showTyping() {
    hideTyping();
    typingEl = document.createElement('div');
    typingEl.className = 'egobot-msg assistant egobot-typing';
    typingEl.textContent = 'EgoLifter is thinking…';
    messagesEl.appendChild(typingEl);
    scrollToBottom();
  }
  function hideTyping() {
    if (typingEl) { typingEl.remove(); typingEl = null; }
  }

  /* ── Send a turn ── POST /egolifter/chat { chatId, message } → { chatId, content } */
  async function sendMessage() {
    const text = inputEl.value.trim();
    if (!text) return;

    messages.push({ role: 'user', content: text });
    renderMessage('user', text);
    inputEl.value = '';
    autoGrow();
    scrollToBottom();
    showTyping();
    sendBtn.disabled = true;

    try {
      const data = await API.apiFetch('/egolifter/chat', {
        method: 'POST',
        body: { chatId: currentChatId, message: text },
      });
      hideTyping();
      if (data && data.chatId) currentChatId = data.chatId;
      const reply = data ? data.content : '';
      messages.push({ role: 'assistant', content: reply });
      renderMessage('assistant', reply);
      scrollToBottom();
      loadChats(); // surface the new/updated chat in the history panel
    } catch (err) {
      hideTyping();
      showStatus(err.message, false);
    } finally {
      sendBtn.disabled = false;
      inputEl.focus();
    }
  }

  /* ── History ── GET /egolifter/chats → [{ chatId, title, createdAt, updatedAt }] */
  async function loadChats() {
    try {
      chats = await API.apiFetch('/egolifter/chats') || [];
      renderHistory();
    } catch (err) {
      historyEl.innerHTML = '<div class="egobot-history-empty">Could not load chats</div>';
    }
  }

  function renderHistory() {
    historyEl.innerHTML = '';
    if (!chats.length) {
      historyEl.innerHTML = '<div class="egobot-history-empty">No chats yet</div>';
      return;
    }
    chats.forEach(c => {
      const item = document.createElement('div');
      item.className = 'egobot-history-item' + (c.chatId === currentChatId ? ' active' : '');

      const title = document.createElement('span');
      title.className = 'egobot-history-name';
      title.textContent = (c.title && c.title.trim()) ? c.title : '(untitled)';
      item.appendChild(title);

      const del = document.createElement('button');
      del.type = 'button';
      del.className = 'egobot-history-del';
      del.textContent = '✕';
      del.title = 'Delete chat';
      del.addEventListener('click', e => {
        e.stopPropagation(); // don't open the chat we're deleting
        deleteChat(c.chatId);
      });
      item.appendChild(del);

      item.addEventListener('click', () => openChat(c.chatId));
      historyEl.appendChild(item);
    });
  }

  /* ── Open a chat ── GET /egolifter/chats/{id}/messages → [{ role, content, createdAt }] */
  async function openChat(id) {
    if (id === currentChatId && messages.length) return;
    try {
      const msgs = await API.apiFetch('/egolifter/chats/' + encodeURIComponent(id) + '/messages');
      messages = (msgs || []).map(m => ({ role: m.role, content: m.content }));
      currentChatId = id;
      renderAll();
      renderHistory(); // refresh the active highlight
    } catch (err) {
      showStatus(err.message, false);
    }
  }

  /* ── Delete a chat ── DELETE /egolifter/chats/{id} → 204 */
  async function deleteChat(id) {
    try {
      await API.apiFetch('/egolifter/chats/' + encodeURIComponent(id), { method: 'DELETE' });
    } catch (err) {
      showStatus(err.message, false);
      return;
    }
    chats = chats.filter(c => c.chatId !== id);
    if (id === currentChatId) newChat(); // the open chat was deleted → reset the pane
    renderHistory();
  }

  function newChat() {
    messages = [];
    currentChatId = 0;
    messagesEl.innerHTML = '';
    renderHistory(); // drop the active highlight
    inputEl.value = '';
    autoGrow();
    inputEl.focus();
  }

  // Grow the textarea with its content, up to the CSS max-height.
  function autoGrow() {
    inputEl.style.height = 'auto';
    inputEl.style.height = Math.min(inputEl.scrollHeight, 160) + 'px';
  }

  /* ── Wiring ── */
  sendBtn.addEventListener('click', sendMessage);
  newChatBtn.addEventListener('click', newChat);
  inputEl.addEventListener('input', autoGrow);
  inputEl.addEventListener('keydown', e => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendMessage();
    }
  });

  loadChats();
  inputEl.focus();
}

/* ── Settings (operator profile stats) ── */
// Settings: view and edit personal stats (name, height, weight). Loads the
// current profile from GET /profile and saves via PUT /profile.
function renderSettings(container) {
  container.innerHTML = '';

  const user = API.getUser();
  const email = (user && user.email) ? user.email : '—';

  const panel = document.createElement('div');
  panel.className = 'settings-panel';
  panel.innerHTML = `
    <h2 class="panel-title">// SETTINGS</h2>
    <div class="calories-today">// OPERATOR: ${email}</div>
    <div class="settings-body"><div class="list-loading">// LOADING…</div></div>
  `;
  container.appendChild(panel);

  const body = panel.querySelector('.settings-body');

  API.apiFetch('/profile').then(profile => {
    buildSettingsForm(body, profile || {});
  }).catch(err => {
    body.innerHTML = '';
    showStatus(err.message, false);
  });
}

function buildSettingsForm(container, profile) {
  container.innerHTML = '';

  const form = document.createElement('form');
  form.className = 'create-food-form';

  // First/last name on the top line, height/weight on the next line.
  const rows = [
    [
      { id: 'first_name', label: 'First Name',  type: 'text',   placeholder: 'e.g. Sam', flex: 1 },
      { id: 'last_name',  label: 'Last Name',   type: 'text',   placeholder: 'e.g. V',   flex: 1 },
    ],
    [
      { id: 'height_cm',  label: 'Height (cm)', type: 'number', placeholder: '0',        flex: 1 },
      { id: 'weight_kg',  label: 'Weight (kg)', type: 'number', placeholder: '0',        flex: 1 },
    ],
  ];

  const fieldHTML = f => `
    <div class="field" style="flex:${f.flex}">
      <label>${f.label}</label>
      <input type="${f.type}" data-field="${f.id}" placeholder="${f.placeholder}" ${f.type === 'number' ? 'min="0" step="0.1"' : ''} />
    </div>
  `;
  const rowsHTML = rows.map(row => `<div class="create-fields">${row.map(fieldHTML).join('')}</div>`).join('');

  form.innerHTML = `
    ${rowsHTML}
    <div class="nutrition-actions">
      <button class="log-meal-btn" type="submit">SAVE</button>
    </div>
  `;

  container.appendChild(form);

  // Prefill from the loaded profile (blank when never saved / zero values).
  form.querySelector('[data-field="first_name"]').value = profile.first_name || '';
  form.querySelector('[data-field="last_name"]').value = profile.last_name || '';
  if (profile.height_cm) form.querySelector('[data-field="height_cm"]').value = profile.height_cm;
  if (profile.weight_kg) form.querySelector('[data-field="weight_kg"]').value = profile.weight_kg;

  form.addEventListener('submit', async e => {
    e.preventDefault();
    const body = {
      first_name: form.querySelector('[data-field="first_name"]').value.trim(),
      last_name:  form.querySelector('[data-field="last_name"]').value.trim(),
      height_cm:  +form.querySelector('[data-field="height_cm"]').value,
      weight_kg:  +form.querySelector('[data-field="weight_kg"]').value,
    };
    const btn = form.querySelector('.log-meal-btn');
    btn.disabled = true;
    try {
      const saved = await API.apiFetch('/profile', { method: 'PUT', body });
      showStatus('Profile saved.', true);
      buildSettingsForm(container, saved);
    } catch (err) {
      showStatus(err.message, false);
    } finally {
      btn.disabled = false;
    }
  });
}

/* ── Render default section on load ── */
renderSection('nutrition');

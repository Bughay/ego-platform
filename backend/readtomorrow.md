# Read Tomorrow — Security & Code Review Notes

A study guide for the issues found in the backend during the code review. The goal here is
**learning**, not copy-paste. Each section explains *what* is wrong, shows the *real* code from
this repo (before), a *suggested* version (after), and most importantly *why* the change matters
and what the underlying concept is.

Nothing in the codebase has been changed. These are notes for you to work through yourself.

Priority order to tackle them:

1. **H2** — Recipe cross-tenant access (real authorization gap)
2. **H1** — Refresh-token expiry config bug
3. **M1 / M2** — Cookie `Secure` flag + real token revocation
4. **M3** — Request body size limit + security headers
5. **L1 – L3** — Hardening & cleanup

---

## The good news first

Before the problems, understand what the codebase already does *right*, so you can keep doing it:

- **No SQL injection.** Every query is parameterized (`$1`, `$2`, `sqlc.arg(...)`), and you use
  sqlc to generate Go from SQL. There is no string concatenation building SQL anywhere.
- **Passwords are hashed** with bcrypt at cost 12 (`internal/auth/service.go:50`) and the
  `Password` field is hidden from JSON output.
- **Login doesn't leak which emails exist** — it returns the same generic
  "invalid email or password" whether the user is missing or the password is wrong
  (`internal/auth/service.go:75-82`). This prevents *user enumeration*.
- **JWT validation rejects the `alg=none` / algorithm-confusion attack** and separates access vs
  refresh tokens.
- **Almost every query is already scoped by `user_id`** from the token, which is exactly how you
  prevent users reading each other's data.

The findings below are the exceptions.

---

# HIGH severity

## H1 — Refresh tokens use the wrong expiry setting

**File:** `internal/shared/config/config.go:65-72`

### The problem
The access token and the refresh token are supposed to have *different* lifetimes. An access
token should be short-lived (minutes/an hour) because it's sent on every request and is the most
exposed. A refresh token is long-lived (days/weeks) and is used only to mint new access tokens.

But both read the **same** environment variable, `JWT_EXPIRY_HOURS`:

### Before
```go
jwtExpiry, _ := strconv.Atoi(os.Getenv("JWT_EXPIRY_HOURS"))
if jwtExpiry == 0 {
    jwtExpiry = 1
}
refreshTokenExpiry, _ := strconv.Atoi(os.Getenv("JWT_EXPIRY_HOURS")) // <-- same env var!
if refreshTokenExpiry == 0 {
    refreshTokenExpiry = 24
}
```

There are actually **two** bugs tangled together here:

1. **Same variable.** If you set `JWT_EXPIRY_HOURS=2` to make access tokens last 2 hours, you
   *also* silently change `refreshTokenExpiry` to 2. You can never configure them independently.
2. **Unit confusion.** The struct field is named `RefreshExpiryDays` (`config.go:34`), and at the
   call site it's multiplied by 24:
   ```go
   // cmd/server/main.go:40
   jwtManager := auth.NewManager(cfg.JWT.Secret, cfg.JWT.AccessExpiryHours, cfg.JWT.RefreshExpiryDays*24)
   ```
   `NewManager`'s third argument is **hours**. So a value that came from a variable literally
   named `..._HOURS`, stored in a field named `...Days`, gets multiplied by 24 as if it were days.
   The default `24` happens to land on `24 * 24 = 576` hours ≈ 24 days, which *looks* plausible but
   is accidental, not intended.

### After
Give the refresh token its own variable and keep the unit consistent end-to-end. Pick one unit
("days") and convert to hours exactly once.

```go
// access token: hours
accessExpiry, _ := strconv.Atoi(os.Getenv("JWT_EXPIRY_HOURS"))
if accessExpiry == 0 {
    accessExpiry = 1
}

// refresh token: days (its own variable)
refreshExpiryDays, _ := strconv.Atoi(os.Getenv("JWT_REFRESH_EXPIRY_DAYS"))
if refreshExpiryDays == 0 {
    refreshExpiryDays = 7 // a week is a common default
}
```

The field name `RefreshExpiryDays` now actually means days, and `main.go`'s `* 24` is correct.

### What to learn
- **Access vs refresh tokens exist precisely so they can have different lifetimes.** Coupling them
  defeats the point of having two tokens.
- **Name things by their unit** (`...Days` vs `...Hours`) and convert at exactly one boundary.
  Mixed units that "happen to work" are a classic source of bugs that surface much later.
- `strconv.Atoi(...)`'s error is being ignored with `_`. That's acceptable for "use default on
  empty/garbage", but be aware: `JWT_EXPIRY_HOURS=abc` silently becomes the default rather than
  failing loudly. For security-relevant config, failing loudly is often safer.

---

## H2 — A user can put another user's food into their recipe (IDOR)

**Files:**
- `internal/egolifter/recipe/validation.go:23-33`
- `internal/egolifter/recipe/repository.go:194-206` (`insertIngredients`)
- `sql/queries/recipe.sql:35-38` (`CreateRecipeIngredient`), `:40-54` (`ListRecipeIngredients`)

### The problem
This is the most important finding because it's a genuine **authorization** gap, not just
hardening. The class of bug is called **IDOR** — *Insecure Direct Object Reference*, also called
*broken object-level authorization*. It means: the server trusts an ID supplied by the client
and acts on it *without checking that the client is allowed to touch that object*.

When you create a recipe, each ingredient carries a `food_id` chosen by the client. The only
validation is "is it non-empty":

### Before
```go
// validation.go — the ONLY check on food_id
for i, ing := range ingredients {
    if strings.TrimSpace(ing.FoodID) == "" {
        return fmt.Errorf("validation: ingredient %d: food_id is required", i)
    }
    // ... weight checks only. Nothing verifies the food belongs to this user.
}
```

```go
// repository.go — inserts whatever food_id the client sent
func insertIngredients(ctx context.Context, q *db.Queries, recipeID string, ingredients []IngredientInput) error {
    for _, ing := range ingredients {
        if _, err := q.CreateRecipeIngredient(ctx, db.CreateRecipeIngredientParams{
            RecipeID: recipeID,
            FoodID:   ing.FoodID, // <-- no ownership check
            WeightG:  ing.WeightG,
            Notes:    toPgText(ing.Notes),
        }); err != nil {
            return fmt.Errorf("insert ingredient (food %s): %w", ing.FoodID, err)
        }
    }
    return nil
}
```

The database foreign key only enforces that `food_id` points at *some* row in `food` — it does
**not** care whose food it is. Then when the recipe is read back, the query joins `food` with no
owner filter at all:

```sql
-- recipe.sql: ListRecipeIngredients — note there is NO user_id condition
SELECT ri.id, ri.recipe_id, ri.food_id, ri.weight_g, ri.notes,
       f.name AS food_name, f.calories_100, f.protein_100, f.carbohydrates_100, f.fat_100
FROM recipe_ingredients ri
JOIN food f ON f.id = ri.food_id
WHERE ri.recipe_id = $1
ORDER BY ri.created_at;
```

**Attack:** if attacker learns (or guesses) another user's `food_id`, they can reference it in
their own recipe and read back that food's name and macros. The practical risk is *limited* today
because `food_id` is a random UUID (hard to guess), but "hard to guess" is not an authorization
control — it's obscurity. If a UUID ever leaks (logs, a shared screenshot, an API response), the
gap becomes exploitable.

### After
Verify every `food_id` belongs to the requesting user *before* inserting. The cleanest way that
fits your sqlc convention is to add a query that returns which of the supplied IDs the user
actually owns, then compare counts.

Add to `sql/queries/food.sql`:
```sql
-- name: CountOwnedFoods :one
SELECT count(*) FROM food
WHERE user_id = $1
  AND id = ANY($2::uuid[]);
```

Then validate inside the transaction, where you already have `userID`:
```go
func insertIngredients(ctx context.Context, q *db.Queries, userID, recipeID string, ingredients []IngredientInput) error {
    ids := make([]string, len(ingredients))
    for i, ing := range ingredients {
        ids[i] = ing.FoodID
    }

    owned, err := q.CountOwnedFoods(ctx, db.CountOwnedFoodsParams{UserID: userID, Column2: ids})
    if err != nil {
        return fmt.Errorf("verify food ownership: %w", err)
    }
    if int(owned) != len(ids) {
        // at least one food_id is missing or belongs to someone else
        return fmt.Errorf("validation: one or more ingredients reference a food you do not own")
    }

    for _, ing := range ingredients {
        if _, err := q.CreateRecipeIngredient(ctx, db.CreateRecipeIngredientParams{
            RecipeID: recipeID,
            FoodID:   ing.FoodID,
            WeightG:  ing.WeightG,
            Notes:    toPgText(ing.Notes),
        }); err != nil {
            return fmt.Errorf("insert ingredient (food %s): %w", ing.FoodID, err)
        }
    }
    return nil
}
```
(You'd pass `userID` through from `Create`/`Update`, which already have it.)

Also defend at the read layer so the join can never surface someone else's food. Add the owner
filter to `ListRecipeIngredients` the same way `GetRecipeFoods` already does it:
```sql
FROM recipe_ingredients ri
JOIN food f ON f.id = ri.food_id
JOIN recipe r ON r.id = ri.recipe_id
WHERE ri.recipe_id = sqlc.arg(recipe_id)::uuid
  AND r.user_id   = sqlc.arg(user_id)::uuid
ORDER BY ri.created_at;
```

> Remember: after editing any `sql/queries/*.sql` you must run `sqlc generate`. Never run the
> migrations yourself — that's the developer's job per the project rules.

### What to learn
- **IDOR is about *authorization*, not authentication.** The attacker is fully logged in as
  themselves. The bug is the server acting on an ID without asking "is this *your* object?"
- **The fix belongs at every layer that takes an ID from the client.** Validate on write *and*
  scope on read. Defense in depth: even if one check is bypassed, the other catches it.
- **Random UUIDs are not an access-control mechanism.** Unguessability slows an attacker; it
  doesn't authorize them. Always pair object IDs with an explicit owner check.
- Notice `GetRecipeFoods` *already* joins `recipe` and filters `r.user_id` — that's the pattern.
  `ListRecipeIngredients` simply forgot it. Consistency across queries matters.

---

# MEDIUM severity

## M1 — Cookies are sent without the `Secure` flag (and a CSRF angle)

**Files:** `internal/auth/handler.go:66-74` (login), `:88-96` (logout), `:122-130` (refresh);
`internal/auth/middleware.go:56-64`

### The problem
The refresh-token cookie is created with `Secure: false` everywhere:

### Before
```go
http.SetCookie(w, &http.Cookie{
    Name:     "refresh_token",
    Value:    refreshToken,
    Path:     "/",
    HttpOnly: true,
    Secure:   false, // <-- cookie will travel over plain HTTP
    SameSite: http.SameSiteLaxMode,
    MaxAge:   86400 * 7,
})
```

`Secure: false` means the browser will send this cookie over an unencrypted `http://` connection.
On any shared/hostile network, the refresh token — which is effectively a long-lived key to the
account — can be sniffed in transit.

There's a second, subtler issue. `/auth/refresh` is a **state-changing POST whose identity comes
entirely from the cookie** the browser attaches automatically. That's the setup for **CSRF**
(Cross-Site Request Forgery): a malicious page can cause the visitor's browser to fire a POST to
your `/auth/refresh`, and the browser will attach the cookie. `SameSite=Lax` blocks the worst
cases but is weaker than `Strict`.

### After
Set `Secure` based on environment (so local HTTP dev still works, but production over HTTPS is
protected), and prefer `Strict` for a refresh cookie that's only ever used by your own site:

```go
http.SetCookie(w, &http.Cookie{
    Name:     "refresh_token",
    Value:    refreshToken,
    Path:     "/",
    HttpOnly: true,
    Secure:   cfg.Server.SecureCookies, // true in prod, false in local dev
    SameSite: http.SameSiteStrictMode,  // refresh is same-site only
    MaxAge:   86400 * 7,
})
```
You'd add a `SecureCookies bool` to config (e.g. from `COOKIE_SECURE=true`) and thread it to the
handler. Because the cookie is set in several places, this is a good candidate for a small helper
like `setRefreshCookie(w, value, cfg)` so the policy lives in exactly one spot.

### What to learn
- **`HttpOnly`** stops JavaScript from reading the cookie (defends against XSS stealing it) — you
  already have this, good. **`Secure`** stops it being sent over plaintext HTTP (defends against
  network sniffing). They're different protections; you want both.
- **`SameSite`** is your first line of CSRF defense. `Strict` for cookies that only your own site
  needs; `Lax` when you need the cookie to survive top-level navigations from other sites.
- Any cookie-authenticated, state-changing endpoint deserves CSRF thought. For an SPA, a common
  pattern is to *not* rely on the cookie alone for such actions (e.g. also require a header the
  browser won't auto-attach cross-site).

---

## M2 — Logout and refresh don't actually invalidate the old token

**File:** `internal/auth/service.go:101-139`

### The problem
JWTs are **stateless**: the server doesn't store them, it just checks the signature and expiry.
That's great for scaling but has a consequence — *you cannot "cancel" a JWT* unless you add some
server-side state. Right now both logout and rotation are no-ops against a stolen token.

### Before
```go
func (s *authService) Logout(ctx context.Context, refreshToken string) error {
    // Validate the token is even real before accepting logout
    _, err := s.jwtManager.ValidateRefresh(refreshToken)
    if err != nil {
        return fmt.Errorf("auth: invalid token")
    }
    return nil // <-- nothing is actually revoked
}
```

And `Refresh` even claims rotation happens, but nothing enforces it:
```go
// Issue NEW tokens (rotation — old refresh token is now invalid)
```
The old refresh token keeps working until it naturally expires. So "log out" doesn't really log
you out, and a leaked refresh token stays valid for its full lifetime even after you "rotate".

### After (the concept)
To truly revoke, you need server-side state. Two common approaches:

1. **A denylist of revoked token IDs.** Give each refresh token a unique `jti` (JWT ID) claim. On
   logout/rotation, store that `jti` in a `revoked_tokens` table (with its expiry, so you can prune
   old rows). `ValidateRefresh` then also checks "is this `jti` revoked?".

2. **An allowlist (session table).** Store each issued refresh token (or its hash) in a
   `refresh_tokens` table. A token is valid only if it's present. Logout deletes the row; rotation
   deletes the old row and inserts the new one. This is stronger — a token not in the table is dead.

Sketch of the allowlist idea:
```go
func (s *authService) Logout(ctx context.Context, refreshToken string) error {
    claims, err := s.jwtManager.ValidateRefresh(refreshToken)
    if err != nil {
        return fmt.Errorf("auth: invalid token")
    }
    // delete this session so the token can never be used again
    return s.userRepo.DeleteRefreshSession(ctx, claims.TokenID)
}
```

### What to learn
- **Stateless tokens trade revocability for scalability.** If you need real logout, real
  rotation, or "log out everywhere", you must reintroduce *some* server state — there is no way
  around it with pure JWTs.
- **Don't let a comment lie.** The `// old refresh token is now invalid` comment describes
  behavior the code doesn't implement. Comments that claim a security property the code lacks are
  dangerous because future readers trust them.
- This is a design decision, not a one-line fix. It's fine to ship without it *if* you document
  that logout is best-effort — but know the tradeoff you're making.

---

## M3 — No request body size limit, and no security headers

**Files:** `cmd/server/main.go:113-118`, every handler that does `json.NewDecoder(r.Body).Decode(...)`
(e.g. `internal/auth/handler.go:27`), `internal/shared/lib/middleware.go`

### The problem — unbounded body reads
Every handler decodes the body with no size cap:

### Before
```go
var req RegisterRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    ...
}
```
`ReadTimeout` bounds how *long* a request may take, but not how *big* it may be. A client can send
a multi-megabyte (or gigabyte) body and the server will read it into memory. The AI chat endpoints
are worst-case: they persist an arbitrarily long prompt straight into the database.

### After
Wrap the body with `http.MaxBytesReader`. It caps the read and returns an error past the limit:
```go
r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MiB cap

var req RegisterRequest
if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
    // an over-limit body lands here; respond 413 Request Entity Too Large
    lib.WriteError(w, http.StatusRequestEntityTooLarge, "request body too large")
    return
}
```
Even cleaner: do it once in middleware (with a larger cap for the chat routes if needed) so you
don't repeat it in every handler.

### The problem — missing security headers
Responses set no defensive headers. Add a tiny middleware (sits next to `CORS` in
`internal/shared/lib/middleware.go`):
```go
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("Referrer-Policy", "no-referrer")
        // Content-Security-Policy is worth adding once you know your asset sources.
        next.ServeHTTP(w, r)
    })
}
```
Then chain it in `main.go`:
```go
Handler: lib.SecurityHeaders(lib.CORS(lib.RequestLogger(logger)(mux))),
```

### What to learn
- **Always bound untrusted input.** Time limits and size limits are different defenses; you need
  both. Unbounded reads are a cheap denial-of-service: one client can exhaust server memory.
- **`nosniff`** stops browsers from MIME-sniffing a response into something executable;
  **`X-Frame-Options: DENY`** stops clickjacking by forbidding your pages from being framed. These
  are one-line, near-zero-risk hardening wins.

---

# LOW severity

## L1 — Internal error details leak to the client

**Files:** `internal/auth/middleware.go:34`, and the decode paths in handlers
(e.g. `internal/auth/handler.go:29, 55`).

### The problem
Some responses hand the raw Go error string to the client.

### Before
```go
// middleware.go — leaks parser/validation internals like "auth: invalid token: ..."
claims, err := m.ValidateAccess(parts[1])
if err != nil {
    lib.WriteError(w, http.StatusUnauthorized, err.Error())
    return
}
```
```go
// handler.go — leaks the JSON decoder's internal message
lib.WriteError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
```
Internal error text can reveal library names, struct field expectations, or parsing details that
help an attacker map your system. (Your 500-level handlers already avoid this — they return generic
strings and log the detail. These two spots are the exceptions.)

### After
Return a fixed, generic message to the client; log the detail server-side (the handlers already
have a `log` in scope):
```go
claims, err := m.ValidateAccess(parts[1])
if err != nil {
    lib.WriteError(w, http.StatusUnauthorized, "invalid or expired token")
    return
}
```
```go
log.WarnContext(r.Context(), "invalid request body", "error", err) // detail stays in logs
lib.WriteError(w, http.StatusBadRequest, "malformed request body")  // generic to client
```

### What to learn
- **Errors are for two different audiences.** Operators (you) want the full detail in logs;
  clients should get the *minimum* needed to correct their request. Leaking internals is low-risk
  on its own but is free reconnaissance for an attacker.

---

## L2 — The AI agent can run a long time and tie up resources

**Files:** `pkg/agent/deepseek/react.go:13-19` (`maxAgentIterations = 100`),
`pkg/agent/deepseek/deepseek.go` (300s HTTP client timeout), vs `WriteTimeout: 30s`.

### The problem
`POST /egolifter/chat` runs the ReAct loop, which can iterate up to 100 times, each making an
upstream LLM call (up to 300s) and DB calls for tools. The HTTP server's `WriteTimeout` is 30s, so
the agent can keep working on a request whose response can no longer be written — burning a
goroutine and a DB connection. A few abusive prompts could exhaust the pool.

### After
Put an explicit deadline on the agent run sized to what you're willing to spend, and consider
limiting how many AI requests one user can run at once:
```go
ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
defer cancel()
resp, err := agent.Run(ctx) // react.go already honors ctx cancellation via sleepCtx
```
The loop already checks `ctx.Err()` each iteration and uses `sleepCtx`, so a deadline will stop it
promptly — you just need to *set* one.

### What to learn
- **Every long-running, externally-triggered operation needs a budget.** A `context` deadline is
  how Go expresses "give up after this long". Without one, slow upstreams become *your* outage.
- **Bounded concurrency** (e.g. a per-user semaphore) protects shared resources like the DB pool
  from a single noisy user.

---

## L3 — Dead code and inconsistent error handling

**Files:** `internal/egolifter/nutrition/validation.go:67-72`,
`internal/egolifter/nutrition/meal_handler.go:104-114`.

### The problem
`validateFoodID` is defined but never called — dead code that readers must puzzle over:
```go
func validateFoodID(id string) error {
    if strings.TrimSpace(id) == "" {
        return fmt.Errorf("validation: food id is required")
    }
    return nil
}
```
Separately, `MealHandler.ViewMeal` doesn't branch on validation errors the way the food handler
does, so a validation error there would be reported as a 500 instead of a 4xx.

### After
Either wire `validateFoodID` into the handlers that take an ID, or delete it. And mirror the
food handler's `isValidationErr(err)` branch in `ViewMeal` so client mistakes return the right
status. These are maintainability nits, not security — but small inconsistencies are where real
bugs hide later.

### What to learn
- **Dead code is a cost, not a freebie.** It misleads readers and rots silently. Delete it or use
  it.
- **Handle the same error class the same way everywhere.** When one handler maps validation errors
  to 422 and a sibling maps them to 500, the inconsistency *is* the bug.

---

## How to verify your changes

After you work through these:

```bash
go build ./...     # compiles
go vet ./...       # catches common mistakes
go test ./...      # run the suite
sqlc generate      # ONLY if you edited sql/queries/*.sql — regenerates internal/shared/db/
```

Do **not** run database migrations — per the project rules, the developer applies those manually.
You may write migration files under `sql/migrations/` in `NNN_name.{up,down}.sql` format and stop.

---

## One-line recap

| # | Issue | Core concept to learn |
|---|-------|-----------------------|
| H1 | Refresh token reads access token's expiry var | Access vs refresh tokens have different lifetimes *on purpose* |
| H2 | Recipe accepts any user's `food_id` | IDOR / broken object-level authorization — always check ownership |
| M1 | Cookies not `Secure`; CSRF on refresh | `HttpOnly` vs `Secure` vs `SameSite` each defend something different |
| M2 | Logout/rotate don't revoke tokens | Stateless JWTs can't be revoked without server-side state |
| M3 | No body size cap / security headers | Bound all untrusted input; cheap hardening headers |
| L1 | Raw errors returned to clients | Detail to logs, generic message to clients |
| L2 | Agent can run unbounded | Every external op needs a `context` deadline |
| L3 | Dead code / inconsistent errors | Consistency and removing rot prevent future bugs |

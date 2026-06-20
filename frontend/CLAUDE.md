# Ego Platform (monorepo)

Multiple apps behind **one authentication** and **one PostgreSQL database**.

```
EgoFinalProject/
├── backend/                  # single Go module (github.com/Bughay/egolifter)
│   ├── cmd/
│   │   ├── server/           # THE HTTP server — mounts every app
│   │   └── agent-cli/        # standalone DeepSeek REPL
│   ├── internal/
│   │   ├── auth/             # the ONE auth (JWT)
│   │   ├── shared/           # config, db (sqlc), lib
│   │   ├── egolifter/        # fitness app (nutrition, recipe, training, analytics, profile)
│   │   └── ego_ai_studio/    # AI Studio HTTP layer (pkg `aistudio`) — scaffold
│   ├── pkg/agent/            # DeepSeek ReAct engine + frontend-builder workflows
│   ├── sql/migrations/       # 001_user.* (fitness) + 002_ai_studio.* (chats/messages)
│   └── .env                  # DATABASE_URL, JWT_SECRET, DEEPSEEKAPIKEY, …
└── frontend/                 # all UIs (static, no build step)
    ├── login/                # shared login (stores JWT in localStorage)
    ├── appcatalog/           # app launcher (one tile per app)
    ├── shared/api.js         # shared API client (Bearer token, requireAuth)
    ├── egolifter/            # fitness UI
    ├── egolifter_landing/
    └── ego_ai_studio/        # AI chat UI (wired to /ai/chat, behind shared auth)
```

## Run

```bash
cd backend
# apply migrations to your Postgres yourself (golang-migrate), then:
go run ./cmd/server          # API on :8080
go run ./cmd/agent-cli       # optional DeepSeek CLI (needs DEEPSEEKAPIKEY)
```

Open `frontend/login/` in a browser (or serve the `frontend/` dir), log in, then
launch an app from the catalog.

## How single auth works

1. `frontend/login/` logs in and stores the JWT via `shared/api.js`.
2. Every app's frontend sends `Authorization: Bearer <token>` to the one backend on `:8080`.
3. `cmd/server/main.go` builds one `auth.Manager` + one `pgxpool.Pool` and wraps every
   app's routes with `jwtManager.Middleware`. Handlers read the user via
   `auth.ClaimsFromContext(r.Context()).UserID`.
4. All per-app tables reference `users(id) ON DELETE CASCADE`.


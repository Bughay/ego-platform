# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

**Ego Platform** — a monorepo housing two apps behind one auth and one PostgreSQL database:

- **EgoLifter** — fitness tracking (nutrition, training, recipes, analytics, profile)
- **Ego AI Studio** — AI chat interface powered by a DeepSeek ReAct agent

The Go backend (`github.com/Bughay/egolifter`) serves all apps on `:8080`. The frontend is static HTML/CSS/JS with no build step.

## Commands

All commands run from `backend/`:

```bash
go run ./cmd/server          # start the API server on :8080
go run ./cmd/agent-cli       # standalone DeepSeek REPL (needs DEEPSEEKAPIKEY)
go build ./...
go test ./...
go test ./internal/egolifter/nutrition/ -run TestXxx -v   # single test
go vet ./...
sqlc generate                # regenerate internal/shared/db/ from sql/queries/
```

Migrations use **golang-migrate** but are applied manually by the developer  never run `migrate up/down` yourself. You may write migration files under `sql/migrations/` in the `NNN_name.{up,down}.sql` format, then stop.



## Architecture

### Backend

```
cmd/
  server/main.go       # builds ONE pgxpool.Pool + ONE auth.Manager; mounts all routes
  agent-cli/main.go    # DeepSeek REPL (oneshot / chat / frontend / frontend-plan / frontend-execute)
internal/
  auth/                # JWT issuing/validation, middleware, login/register/logout/refresh
  shared/
    config/            # env-based config (LoadConfig)
    db/                # sqlc-generated code — DO NOT hand-edit
    lib/               # CORS, RequestLogger, WriteJSON, WriteError
  egolifter/           # fitness domains — each follows handler → service → repository + model
    nutrition/         #   foods + meal consumption logging
    training/          #   workouts + exercises
    recipe/            #   recipes + ingredients
    analytics/         #   date-range summaries (no repository — composes meal + training services)
    profile/           #   user profile
  ego_ai_studio/
    chatbot/           # AI Studio HTTP layer (POST /ai/chat, GET /ai/chats, etc.) — built out
pkg/agent/             # DeepSeek ReAct engine + frontend-builder workflows (importable library)
  deepseek/            # API client, ReAct loop (Agent.Run), tool parsing
  grok/                # Grok API client
  tools/               # frontend file tool implementations (embed frontend_executer.json)
  workflows/           # three-phase frontend pipeline (research → plan → execute)
  prompts/             # system prompt constants
  helper/              # stdin reader, atomic EditFile
sql/
  migrations/          # 001_user.* (fitness schema) + 002_ai_studio.* (chats/messages)
  queries/             # sqlc source-of-truth SQL files
```

### Frontend

```
frontend/
  login/               # shared login — stores JWT in localStorage via shared/api.js
  appcatalog/          # app launcher (one tile per app)
  shared/api.js        # API client — apiFetch (Bearer token), requireAuth, logout
  egolifter/           # fitness UI
  egolifter_landing/   # landing/marketing page
  ego_ai_studio/       # AI chat UI (wired to /ai/chat; set DEMO_MODE = false to enable)
```

All frontends load `../shared/api.js` before their own `script.js`. JWT is stored in `localStorage` under `egolifter_token`. A `401` from the backend clears the token and redirects to `../login/`.



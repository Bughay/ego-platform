# EgoLifter — Backend

## Introduction

EgoLifter is the backend for a cyberpunk-themed fitness companion — think of it as the chrome implant for your training life: every meal, every rep, every recipe gets jacked into the grid and logged. The aesthetic is neon and dystopian, but the data underneath is precise and entirely yours.

The API is built around three pillars:

- **Nutrition tracking** — build your own food database where every food stores its macros per 100 grams (calories, protein, carbohydrates, fat). When you log a meal, you just say what you ate and how many grams; the API computes the macro totals for you automatically.
- **Training tracking** — create reusable workout routines, then log workouts as they happen: each exercise records its name, working weight in kilograms, and reps. View your history for any day or across a date range.
- **Recipes** — compose recipes from foods already in your database, with a gram weight per ingredient, so the nutritional value of a full dish is always derivable from its parts.

Everything is secured with JWT bearer tokens (access + refresh token rotation), and every user is strictly scoped to their own data — your identity comes from the token, never from the request body. Wake up, samurai. We have macros to count.

## Tech Stack

- **Go 1.26** — pure standard library `net/http` with `http.ServeMux` routing, no web framework
- **PostgreSQL** — accessed through a `pgx/v5` connection pool
- **sqlc** — type-safe Go code generated from SQL queries in `sql/queries/`
- **golang-migrate** — schema migrations in `sql/migrations/`
- **JWT** (`golang-jwt/jwt/v5`) — access + refresh token authentication with rotation
- **bcrypt** (`golang.org/x/crypto`) — password hashing
- **godotenv** — environment-based configuration

## Endpoints

All endpoints except the **Auth** group require a valid JWT in the `Authorization: Bearer <token>` header. Users can only access their own data.

### Auth (public)

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/auth/register` | Register a new user account |
| `POST` | `/auth/login` | Log in with credentials, returns access + refresh tokens |
| `POST` | `/auth/logout` | Log out and invalidate the refresh token |
| `POST` | `/auth/refresh` | Exchange a refresh token for a new token pair |

### Food

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/food/create` | Create a food with its per-100g macros (calories, protein, carbs, fat) |
| `GET` | `/food/view` | List all your foods, or fetch a single one with `?id=` |
| `PUT` | `/food/update` | Update an existing food |
| `DELETE` | `/food/delete` | Delete a food by `?id=` |

### Meal

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/meal/create` | Log a consumed meal (food + weight in grams); macro totals are computed automatically |
| `GET` | `/meal/view` | List all your logged meals, or fetch a single one with `?id=` |
| `GET` | `/meal/by-date` | List meals logged between `?date_from=` and `?date_to=` (YYYY-MM-DD, inclusive) |
| `DELETE` | `/meal/del` | Delete a logged meal by `?id=` |

### Recipe

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/recipe/create` | Create a recipe from foods in your database, with a gram weight per ingredient |
| `GET` | `/recipe/view` | List all your recipes, or fetch a single one with `?id=` |
| `PUT` | `/recipe/update` | Update an existing recipe |
| `DELETE` | `/recipe/del` | Delete a recipe by `?id=` |
| `GET` | `/recipe/getfoods` | Get the ingredient foods of a recipe by `?id=` |

### Training

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/training/routine/create` | Create a reusable workout routine |
| `GET` | `/training/routine/view` | List your workout routines |
| `POST` | `/training/log` | Log a workout with its exercises (name, weight in kg, reps) |
| `GET` | `/training/view` | List your workouts, optionally filtered by `?date=` (YYYY-MM-DD) |
| `GET` | `/training/by-date` | List workouts performed between `?date_from=` and `?date_to=` (YYYY-MM-DD, inclusive) |
| `DELETE` | `/training/del` | Delete a workout by `?id=` |

### Analytics

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/analytics/summary` | Combined nutrition + training summary between `?date_from=` and `?date_to=` (YYYY-MM-DD, inclusive): macro totals, daily averages, days logged, workout count, total reps and volume. Either bound defaults to today. |

### Profile

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/profile` | Get the current user's profile stats (first name, last name, height in cm, weight in kg); empty fields if never saved |
| `PUT` | `/profile` | Create or update the current user's profile stats |

## Future Updates

- Add **Redis** for caching
- Add a **chatbot** to ask questions
- Add an **analytics report generator** that uses an LLM to generate insights based on user data, then uses background jobs to email the report to the user

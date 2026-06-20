Create table IF NOT EXISTS "refresh_tokens" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    token_hash text not null unique,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create index IF NOT EXISTS idx_refresh_tokens_user_id ON "refresh_tokens"(user_id);

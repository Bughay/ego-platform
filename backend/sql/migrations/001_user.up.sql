
Create table IF NOT EXISTS  "users" (
    id uuid primary key default gen_random_uuid(),
    email text not null unique,
    password text not null,
    role text not null default 'user',
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create table IF NOT EXISTS  "user_profiles" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null unique references "users"(id) on delete cascade,
    first_name text,
    last_name text,
    date_of_birth date,
    height_cm float,
    weight_kg float,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);
Create table IF NOT EXISTS "refresh_tokens" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    token_hash text not null unique,
    expires_at timestamptz not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create index IF NOT EXISTS idx_refresh_tokens_user_id ON "refresh_tokens"(user_id);


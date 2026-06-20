
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

Create table IF NOT EXISTS  "food" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    name text not null,
    calories_100 float not null,
    protein_100 float not null, 
    carbohydrates_100 float not null,
    fat_100 float not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create table IF NOT EXISTS  "meal" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    name text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create table IF NOT EXISTS  "food_consumed" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    meal_id uuid not null references "meal"(id) on delete cascade,
    food_id uuid not null references "food"(id) on delete cascade,
    weight_g float not null,
    total_calories float not null,
    total_protein float not null,
    total_carbohydrates float not null,
    total_fat float not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create table IF NOT EXISTS  "workout" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    name text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create table IF NOT EXISTS  "workout_routine" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    name text not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

Create Table IF NOT EXISTS  "workout_routine_entries" (
    id uuid primary key default gen_random_uuid(),
    workout_routine_id uuid not null references "workout_routine"(id) on delete cascade,
    name text not null,
    weight_kg float not null,
    reps integer not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()

);
Create Table IF NOT EXISTS  "exercise" (
    id uuid primary key default gen_random_uuid(),
    workout_id uuid not null references "workout"(id) on delete cascade,
    name text not null,
    weight_kg float not null,
    reps integer not null,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()

);

Create table IF NOT EXISTS  "recipe" (
    id uuid primary key default gen_random_uuid(),
    user_id uuid not null references "users"(id) on delete cascade,
    name text not null,
    notes text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);


Create table IF NOT EXISTS  "recipe_ingredients" ( 
    id uuid primary key default gen_random_uuid(),
    recipe_id uuid not null references "recipe"(id) on delete cascade,
    food_id uuid not null references "food"(id) on delete cascade,
    weight_g float not null,
    notes text,
    created_at timestamptz not null default now(),
    updated_at timestamptz not null default now()
);

CREATE TABLE IF NOT EXISTS "egolifter_chats" (
    id          BIGSERIAL PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES "users"(id) ON DELETE CASCADE,
    title       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS "egolifter_messages" (
    id          BIGSERIAL PRIMARY KEY,
    chat_id     BIGINT NOT NULL REFERENCES "egolifter_chats"(id) ON DELETE CASCADE,
    role        VARCHAR(20) NOT NULL,  -- 'user' | 'assistant' | 'system'
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chats_user_id ON "egolifter_chats"(user_id);
CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON "egolifter_messages"(chat_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON "egolifter_messages"(created_at);
ALTER TABLE "meal" ADD CONSTRAINT uq_meal_id_user UNIQUE (id, user_id);

ALTER TABLE "food_consumed"
  ADD CONSTRAINT fk_food_consumed_meal_user
  FOREIGN KEY (meal_id, user_id) REFERENCES "meal"(id, user_id);

CREATE INDEX idx_food_user_id ON "food"(user_id);


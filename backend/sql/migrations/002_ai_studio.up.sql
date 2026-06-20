-- Ego AI Studio: chat conversations and messages.
-- Single auth / single DB: user_id is a uuid referencing the shared users table
-- (was BIGINT in the standalone app), so AI conversations belong to the same
-- users as the fitness app and cascade-delete with them.

CREATE TABLE IF NOT EXISTS "ai_studio_chats" (
    id          BIGSERIAL PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES "users"(id) ON DELETE CASCADE,
    title       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS "ai_studio_messages" (
    id          BIGSERIAL PRIMARY KEY,
    chat_id     BIGINT NOT NULL REFERENCES "ai_studio_chats"(id) ON DELETE CASCADE,
    role        VARCHAR(20) NOT NULL,  -- 'user' | 'assistant' | 'system'
    content     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chats_user_id ON "ai_studio_chats"(user_id);
CREATE INDEX IF NOT EXISTS idx_messages_chat_id ON "ai_studio_messages"(chat_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON "ai_studio_messages"(created_at);
DROP INDEX IF EXISTS idx_messages_created_at;
DROP INDEX IF EXISTS idx_messages_chat_id;
DROP INDEX IF EXISTS idx_chats_user_id;

DROP TABLE IF EXISTS "messages" CASCADE;
DROP TABLE IF EXISTS "chats" CASCADE;

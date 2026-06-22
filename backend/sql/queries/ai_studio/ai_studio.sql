-- name: CreateChat :one
INSERT INTO ai_studio_chats (user_id, title)
VALUES ($1, $2)
RETURNING *;

-- name: GetChatForUser :one
SELECT * FROM ai_studio_chats
WHERE id = $1 AND user_id = $2;

-- name: GetAllChatsForUser :many
SELECT * FROM ai_studio_chats
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: ListMessagesByChat :many
SELECT * FROM ai_studio_messages
WHERE chat_id = $1
ORDER BY created_at ASC, id ASC;

-- name: CreateMessage :one
INSERT INTO ai_studio_messages (chat_id, role, content)
VALUES ($1, $2, $3)
RETURNING *;

-- name: TouchChat :exec
UPDATE ai_studio_chats SET updated_at = now()
WHERE id = $1;

-- name: DeleteChatForUser :execrows
DELETE FROM ai_studio_chats
WHERE id = $1 AND user_id = $2;

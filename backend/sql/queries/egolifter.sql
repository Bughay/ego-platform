-- name: CreateEgolifterChat :one
INSERT INTO egolifter_chats (user_id, title)
VALUES ($1, $2)
RETURNING *;

-- name: GetEgolifterChatForUser :one
SELECT * FROM egolifter_chats
WHERE id = $1 AND user_id = $2;

-- name: CreateEgolifterMessage :one
INSERT INTO egolifter_messages (chat_id, role, content)
VALUES ($1, $2, $3)
RETURNING *;

-- name: TouchEgolifterChat :exec
UPDATE egolifter_chats SET updated_at = now()
WHERE id = $1;

-- name: GetAllEgolifterChatsForUser :many
SELECT * FROM egolifter_chats
WHERE user_id = $1
ORDER BY updated_at DESC;

-- name: ListEgolifterMessagesByChat :many
SELECT * FROM egolifter_messages
WHERE chat_id = $1
ORDER BY created_at ASC, id ASC;

-- name: DeleteEgolifterChatForUser :execrows
DELETE FROM egolifter_chats
WHERE id = $1 AND user_id = $2;

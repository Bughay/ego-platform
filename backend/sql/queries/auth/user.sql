-- name: GetUserProfile :one
SELECT * FROM user_profiles
WHERE user_id = $1;

-- name: UpsertUserProfile :one
INSERT INTO user_profiles (user_id, first_name, last_name, height_cm, weight_kg)
VALUES (sqlc.arg(user_id)::uuid,
        sqlc.arg(first_name),
        sqlc.arg(last_name),
        sqlc.arg(height_cm),
        sqlc.arg(weight_kg))
ON CONFLICT (user_id) DO UPDATE
SET first_name = EXCLUDED.first_name,
    last_name  = EXCLUDED.last_name,
    height_cm  = EXCLUDED.height_cm,
    weight_kg  = EXCLUDED.weight_kg,
    updated_at = now()
RETURNING *;

-- name: CreateRefreshToken :exec
INSERT INTO refresh_tokens (user_id, token_hash, expires_at)
VALUES (sqlc.arg(user_id)::uuid, sqlc.arg(token_hash), sqlc.arg(expires_at));

-- name: DeleteRefreshToken :execrows
-- Returns the number of rows removed. A result of 0 means the token was not in
-- the store (already revoked, already rotated, or never valid), which callers
-- use as the single-use / revocation gate. Expiry is enforced separately by the
-- JWT layer before this is called, so deleting purely by hash is safe.
DELETE FROM refresh_tokens
WHERE token_hash = $1;

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

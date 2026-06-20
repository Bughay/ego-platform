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

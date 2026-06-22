-- name: CreateFood :one
INSERT INTO food (user_id, name, calories_100, protein_100, carbohydrates_100, fat_100)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetFoodByID :one
SELECT * FROM food
WHERE id = $1 AND user_id = $2;

-- name: ListFoods :many
SELECT * FROM food
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: UpdateFood :one
UPDATE food
SET name              = $3,
    calories_100      = $4,
    protein_100       = $5,
    carbohydrates_100 = $6,
    fat_100           = $7,
    updated_at        = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteFood :exec
DELETE FROM food
WHERE id = $1 AND user_id = $2;

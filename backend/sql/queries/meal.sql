-- name: CreateMeal :one
INSERT INTO meal (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: CreateFoodConsumed :one
INSERT INTO food_consumed (user_id, meal_id, food_id, weight_g,
                           total_calories, total_protein, total_carbohydrates, total_fat)
SELECT sqlc.arg(user_id)::uuid,
       sqlc.arg(meal_id)::uuid,
       f.id,
       sqlc.arg(weight_g)::float8,
       (sqlc.arg(weight_g)::float8 / 100) * f.calories_100,
       (sqlc.arg(weight_g)::float8 / 100) * f.protein_100,
       (sqlc.arg(weight_g)::float8 / 100) * f.carbohydrates_100,
       (sqlc.arg(weight_g)::float8 / 100) * f.fat_100
FROM food f
WHERE f.id = sqlc.arg(food_id)::uuid
RETURNING *;

-- name: GetMealByID :one
SELECT m.id, m.user_id, m.name, m.created_at, m.updated_at,
       coalesce(sum(fc.total_calories), 0)::float8      AS total_calories,
       coalesce(sum(fc.total_protein), 0)::float8       AS total_protein,
       coalesce(sum(fc.total_carbohydrates), 0)::float8 AS total_carbohydrates,
       coalesce(sum(fc.total_fat), 0)::float8           AS total_fat
FROM meal m
LEFT JOIN food_consumed fc ON fc.meal_id = m.id
WHERE m.id = $1 AND m.user_id = $2
GROUP BY m.id;

-- name: ListMeals :many
SELECT m.id, m.user_id, m.name, m.created_at, m.updated_at,
       coalesce(sum(fc.total_calories), 0)::float8      AS total_calories,
       coalesce(sum(fc.total_protein), 0)::float8       AS total_protein,
       coalesce(sum(fc.total_carbohydrates), 0)::float8 AS total_carbohydrates,
       coalesce(sum(fc.total_fat), 0)::float8           AS total_fat
FROM meal m
LEFT JOIN food_consumed fc ON fc.meal_id = m.id
WHERE m.user_id = $1
GROUP BY m.id
ORDER BY m.created_at DESC;

-- name: ListMealsByDateRange :many
SELECT m.id, m.user_id, m.name, m.created_at, m.updated_at,
       coalesce(sum(fc.total_calories), 0)::float8      AS total_calories,
       coalesce(sum(fc.total_protein), 0)::float8       AS total_protein,
       coalesce(sum(fc.total_carbohydrates), 0)::float8 AS total_carbohydrates,
       coalesce(sum(fc.total_fat), 0)::float8           AS total_fat
FROM meal m
LEFT JOIN food_consumed fc ON fc.meal_id = m.id
WHERE m.user_id = sqlc.arg(user_id)
  AND m.created_at::date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
GROUP BY m.id
ORDER BY m.created_at DESC;

-- name: DeleteMeal :execrows
WITH deleted_foods AS (
    DELETE FROM food_consumed
    WHERE meal_id = sqlc.arg(id)::uuid AND user_id = sqlc.arg(user_id)::uuid
)
DELETE FROM meal
WHERE id = sqlc.arg(id)::uuid AND user_id = sqlc.arg(user_id)::uuid;

-- name: ListFoodConsumedByMeal :many
SELECT fc.id,
       fc.food_id,
       fc.weight_g,
       fc.total_calories,
       fc.total_protein,
       fc.total_carbohydrates,
       fc.total_fat,
       f.name AS food_name
FROM food_consumed fc
JOIN food f ON f.id = fc.food_id
WHERE fc.meal_id = $1 AND fc.user_id = $2
ORDER BY fc.created_at;

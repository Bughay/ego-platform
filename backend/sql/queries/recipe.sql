-- name: CreateRecipe :one
INSERT INTO recipe (user_id, name, notes)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetRecipeByID :one
SELECT * FROM recipe
WHERE id = $1 AND user_id = $2;

-- name: ListRecipes :many
SELECT * FROM recipe
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: UpdateRecipe :one
UPDATE recipe
SET name       = $3,
    notes      = $4,
    updated_at = now()
WHERE id = $1 AND user_id = $2
RETURNING *;

-- name: DeleteRecipe :execrows
WITH deleted_ingredients AS (
    DELETE FROM recipe_ingredients
    WHERE recipe_id = sqlc.arg(id)::uuid
      AND EXISTS (
          SELECT 1 FROM recipe r
          WHERE r.id = sqlc.arg(id)::uuid AND r.user_id = sqlc.arg(user_id)::uuid
      )
)
DELETE FROM recipe
WHERE id = sqlc.arg(id)::uuid AND user_id = sqlc.arg(user_id)::uuid;

-- name: CreateRecipeIngredient :one
INSERT INTO recipe_ingredients (recipe_id, food_id, weight_g, notes)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListRecipeIngredients :many
SELECT ri.id,
       ri.recipe_id,
       ri.food_id,
       ri.weight_g,
       ri.notes,
       f.name AS food_name,
       f.calories_100,
       f.protein_100,
       f.carbohydrates_100,
       f.fat_100
FROM recipe_ingredients ri
JOIN food f ON f.id = ri.food_id
WHERE ri.recipe_id = $1
ORDER BY ri.created_at;

-- name: GetRecipeFoods :many
SELECT ri.id,
       ri.food_id,
       ri.weight_g,
       ri.notes,
       f.name AS food_name,
       f.calories_100,
       f.protein_100,
       f.carbohydrates_100,
       f.fat_100
FROM recipe_ingredients ri
JOIN food f ON f.id = ri.food_id
JOIN recipe r ON r.id = ri.recipe_id
WHERE ri.recipe_id = sqlc.arg(recipe_id)::uuid
  AND r.user_id = sqlc.arg(user_id)::uuid
ORDER BY ri.created_at;

-- name: DeleteRecipeIngredientsByRecipe :exec
DELETE FROM recipe_ingredients
WHERE recipe_id = $1;

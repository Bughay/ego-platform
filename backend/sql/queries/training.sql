-- name: CreateWorkoutRoutine :one
INSERT INTO workout_routine (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: CreateWorkoutRoutineEntry :one
INSERT INTO workout_routine_entries (workout_routine_id, name, weight_kg, reps)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetWorkoutRoutineByID :one
SELECT * FROM workout_routine
WHERE id = $1 AND user_id = $2;

-- name: ListWorkoutRoutines :many
SELECT * FROM workout_routine
WHERE user_id = $1
ORDER BY created_at DESC;

-- name: ListWorkoutRoutineEntries :many
SELECT * FROM workout_routine_entries
WHERE workout_routine_id = $1
ORDER BY created_at;

-- name: CreateWorkout :one
INSERT INTO workout (user_id, name)
VALUES ($1, $2)
RETURNING *;

-- name: CreateExercise :one
INSERT INTO exercise (workout_id, name, weight_kg, reps)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: ListWorkoutsByDate :many
SELECT * FROM workout
WHERE user_id = $1 AND created_at::date = $2::date
ORDER BY created_at DESC;

-- name: ListWorkoutsByDateRange :many
SELECT * FROM workout
WHERE user_id = sqlc.arg(user_id)
  AND created_at::date BETWEEN sqlc.arg(date_from)::date AND sqlc.arg(date_to)::date
ORDER BY created_at DESC;

-- name: DeleteWorkout :execrows
WITH deleted_exercises AS (
    DELETE FROM exercise
    WHERE workout_id = sqlc.arg(id)::uuid
      AND EXISTS (
          SELECT 1 FROM workout w
          WHERE w.id = sqlc.arg(id)::uuid AND w.user_id = sqlc.arg(user_id)::uuid
      )
)
DELETE FROM workout
WHERE id = sqlc.arg(id)::uuid AND user_id = sqlc.arg(user_id)::uuid;

-- name: ListExercisesByWorkout :many
SELECT * FROM exercise
WHERE workout_id = $1
ORDER BY created_at;

package workflows

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Bughay/egolifter/internal/egolifter/analytics"
	"github.com/Bughay/egolifter/internal/egolifter/nutrition"
	"github.com/Bughay/egolifter/internal/egolifter/recipe"
	"github.com/Bughay/egolifter/internal/egolifter/training"
	"github.com/Bughay/egolifter/internal/shared/config"
	"github.com/Bughay/egolifter/pkg/agent/deepseek"
	"github.com/Bughay/egolifter/pkg/agent/helper"
	"github.com/Bughay/egolifter/pkg/agent/prompts"

	egotools "github.com/Bughay/egolifter/pkg/agent/tools/egolifter"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	egolifterModel    = "deepseek-v4-flash"
	egolifterTokens   = 100000
	egolifterPrompt   = prompts.EgolifterAgentPrompt
	egolifterThinking = false
)

// RunEgolifterAgent runs the ReAct DeepSeek agent against the egolifter tools
// for a single user request and returns the agent's final answer.
//
// The caller supplies the assembled domain services and the authenticated
// user's id; the id is captured by the tools so the agent can only ever act on
// that user's own data. memory is the prior conversation turns (oldest first,
// excluding the current userPrompt) so the agent can ask a question on one turn
// and use the user's reply on the next; pass nil for a one-shot run. This
// function owns no database — it is the pure workflow and is easy to drive from
// a test with stub services.
func RunEgolifterAgent(ctx context.Context, svc egotools.Services, userID string, memory []deepseek.Message, userPrompt string) (string, error) {
	toolSchema, err := deepseek.LoadToolsFromData(egotools.SchemaJSON)
	if err != nil {
		return "", fmt.Errorf("load egolifter tools schema: %w", err)
	}

	agent := &deepseek.Agent{
		Model:        egolifterModel,
		SystemPrompt: egolifterPrompt,
		UserPrompt:   userPrompt,
		Memory:       memory,
		Thinking:     egolifterThinking,
		Tools:        toolSchema,
		Registry:     egotools.EgolifterFunctions(ctx, svc, userID),
		SchemaData:   egotools.SchemaJSON,
		MaxTokens:    egolifterTokens,
	}

	result, err := agent.Run(ctx)
	if err != nil {
		return "", fmt.Errorf("egolifter agent run: %w", err)
	}
	return result.FinishAnswer(), nil
}

// EgolifterAgentREPL is the self-contained entry point: it builds the real
// Postgres-backed meal, training, and analytics services from config, reads one
// request from stdin, runs the agent for the given user, and prints the answer.
func EgolifterAgentREPL(ctx context.Context, userID string) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	pool, err := pgxpool.New(ctx, cfg.Database.DSN)
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer pool.Close()

	mealSvc := nutrition.NewMealService(nutrition.NewMealRepository(pool))
	foodSvc := nutrition.NewNutritionService(nutrition.NewFoodRepository(pool))
	trainingSvc := training.NewTrainingService(training.NewTrainingRepository(pool))
	analyticsSvc := analytics.NewAnalyticsService(mealSvc, trainingSvc)
	recipeSvc := recipe.NewRecipeService(recipe.NewRecipeRepository(pool))
	svc := egotools.Services{
		Meal:      mealSvc,
		Food:      foodSvc,
		Training:  trainingSvc,
		Analytics: analyticsSvc,
		Recipe:    recipeSvc,
	}

	userPrompt, err := helper.Input("EgoLifter AI — what would you like to do? ")
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	slog.Info("egolifter agent starting", "model", egolifterModel, "user", userID)
	answer, err := RunEgolifterAgent(ctx, svc, userID, nil, userPrompt)
	if err != nil {
		return err
	}

	fmt.Println("\n=== Agent finished ===")
	fmt.Println(answer)
	return nil
}

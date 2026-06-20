package main

import (
	"context"
	"net/http"
	"time"

	"github.com/Bughay/egolifter/internal/auth"
	aistudio "github.com/Bughay/egolifter/internal/ego_ai_studio/chatbot"
	"github.com/Bughay/egolifter/internal/egolifter/analytics"
	"github.com/Bughay/egolifter/internal/egolifter/bot"
	"github.com/Bughay/egolifter/internal/egolifter/nutrition"
	"github.com/Bughay/egolifter/internal/egolifter/profile"
	"github.com/Bughay/egolifter/internal/egolifter/recipe"
	"github.com/Bughay/egolifter/internal/egolifter/training"
	"github.com/Bughay/egolifter/internal/shared/config"
	"github.com/Bughay/egolifter/internal/shared/lib"
	egotools "github.com/Bughay/egolifter/pkg/agent/tools/egolifter"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic("failed to load config: " + err.Error())
	}

	logger := lib.NewLogger(cfg.Server.LogFormat, cfg.Server.LogLevel)

	pool, err := pgxpool.New(context.Background(), cfg.Database.DSN)
	if err != nil {
		logger.Error("failed to connect to database", "error", err)
		panic("failed to connect to database")
	}
	defer pool.Close()

	mux := http.NewServeMux()

	// Auth module: handler -> service -> repository
	jwtManager := auth.NewManager(cfg.JWT.Secret, cfg.JWT.AccessExpiryHours, cfg.JWT.RefreshExpiryDays*24)
	userRepo := auth.NewUserRepository(pool)
	refreshTokenRepo := auth.NewRefreshTokenRepository(pool)
	authSvc := auth.NewAuthService(userRepo, refreshTokenRepo, jwtManager)
	authHandler := auth.NewAuthHandler(authSvc, logger)
	mux.HandleFunc("POST /auth/register", authHandler.Register)
	mux.HandleFunc("POST /auth/login", authHandler.Login)
	mux.HandleFunc("POST /auth/logout", authHandler.Logout)
	mux.HandleFunc("POST /auth/refresh", authHandler.Refresh)

	// Nutrition module: handler -> service -> repository (JWT-protected)
	foodRepo := nutrition.NewFoodRepository(pool)
	nutritionSvc := nutrition.NewNutritionService(foodRepo)
	nutritionHandler := nutrition.NewNutritionHandler(nutritionSvc, logger)
	nutritionHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// Meal endpoints (nutrition module, JWT-protected)
	mealRepo := nutrition.NewMealRepository(pool)
	mealSvc := nutrition.NewMealService(mealRepo)
	mealHandler := nutrition.NewMealHandler(mealSvc, logger)
	mealHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// Recipe module: handler -> service -> repository (JWT-protected)
	recipeRepo := recipe.NewRecipeRepository(pool)
	recipeSvc := recipe.NewRecipeService(recipeRepo)
	recipeHandler := recipe.NewRecipeHandler(recipeSvc, logger)
	recipeHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// Training module: handler -> service -> repository (JWT-protected)
	trainingRepo := training.NewTrainingRepository(pool)
	trainingSvc := training.NewTrainingService(trainingRepo)
	trainingHandler := training.NewTrainingHandler(trainingSvc, logger)
	trainingHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// Analytics module: composes the meal and training services to summarize a
	// date range (no repository of its own, JWT-protected).
	analyticsSvc := analytics.NewAnalyticsService(mealSvc, trainingSvc)
	analyticsHandler := analytics.NewAnalyticsHandler(analyticsSvc, logger)
	analyticsHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// Profile module: handler -> service -> repository (JWT-protected)
	profileRepo := profile.NewProfileRepository(pool)
	profileSvc := profile.NewProfileService(profileRepo)
	profileHandler := profile.NewProfileHandler(profileSvc, logger)
	profileHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// EgoLifter bot module: handler -> service -> repository (JWT-protected).
	// POST /egolifter/chat runs the DeepSeek ReAct agent (pkg/agent/workflows)
	// over the egolifter tools — reusing the meal, training, and analytics
	// services above — and persists the conversation in egolifter_chats /
	// egolifter_messages.
	botRepo := bot.NewRepository(pool)
	botSvc := bot.NewService(botRepo,
		egotools.Services{
			Meal:      mealSvc,
			Food:      nutritionSvc,
			Training:  trainingSvc,
			Analytics: analyticsSvc,
			Recipe:    recipeSvc,
		},
		logger)
	botHandler := bot.NewHandler(botSvc, logger)
	botHandler.RegisterRoutes(mux, jwtManager.Middleware)

	// Ego AI Studio module: handler -> service -> repository (JWT-protected).
	// SCAFFOLD — already plugged into the single auth (jwtManager.Middleware),
	// the single DB pool, and the DeepSeek key from config, but it registers no
	// routes yet. Build it out in internal/ego_ai_studio (HTTP layer) + pkg/agent
	// (the DeepSeek engine), then add routes in RegisterRoutes.
	aiRepo := aistudio.NewRepository(pool)
	aiSvc := aistudio.NewService(aiRepo, cfg.Agent.DEEPSEEKAPIKEY, logger)
	aiHandler := aistudio.NewHandler(aiSvc, logger)
	aiHandler.RegisterRoutes(mux, jwtManager.Middleware)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      lib.CORS(lib.RequestLogger(logger)(mux)),
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
	}

	logger.Info("server listening", "port", cfg.Server.Port)
	if err := srv.ListenAndServe(); err != nil {
		logger.Error("server stopped", "error", err)
	}
}

package main

import (
	"context"
	"fmt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/guarda/backend/internal/config"
	"github.com/guarda/backend/internal/db"
	"github.com/guarda/backend/internal/handler"
	"github.com/guarda/backend/internal/middleware"
	"github.com/guarda/backend/internal/repository"
	"github.com/guarda/backend/internal/service"
	"github.com/guarda/backend/internal/toolcall"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()

	// Connect to database
	pool, err := db.NewPool(ctx, cfg.DBURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer pool.Close()

	// Run migrations
	if err := db.RunMigrations(ctx, pool); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	log.Println("Migrations applied successfully")

	// Initialize repositories
	guestRepo := repository.NewGuestRepo(pool)
	convRepo := repository.NewConversationRepo(pool)
	msgRepo := repository.NewMessageRepo(pool)
	interRepo := repository.NewInteractionRepo(pool)

	// Initialize services
	drugDict, _ := service.NewDrugDictionary("data/drug_dictionary.csv")
	interDataset, _ := service.NewInteractionDataset("data/drug_interactions.csv")
	rxnorm := service.NewRxNormClient()

	var aiService *service.AIService
	if cfg.GeminiAPIKey != "" {
		aiService, err = service.NewAIService(ctx, cfg.GeminiAPIKey)
		if err != nil {
			log.Printf("Warning: Failed to initialize AI service: %v", err)
		} else {
			defer aiService.Close()
			aiService.SetTools(toolcall.GetToolDefinitions())
		}
	} else {
		log.Println("Warning: GEMINI_API_KEY not set, AI features disabled")
	}

	normalizer := service.NewNormalizerService(drugDict, rxnorm, aiService)
	checker := service.NewInteractionChecker(interDataset, aiService)

	// Initialize tool executor
	executor := toolcall.NewExecutor(normalizer, checker)

	// Initialize handlers
	guestHandler := handler.NewGuestHandler(guestRepo)
	chatHandler := handler.NewChatHandler(convRepo, msgRepo, aiService, executor)
	interHandler := handler.NewInteractionHandler(interRepo, checker)

	// Setup router
	r := gin.Default()
	r.Use(middleware.CORS())

	api := r.Group("/api/v1")
	{
		api.POST("/guests", guestHandler.Create)

		// Routes requiring guest auth
		auth := api.Group("")
		auth.Use(middleware.GuestAuth())
		{
			auth.POST("/chat/message", chatHandler.SendMessage)
			auth.POST("/interactions/check", interHandler.Check)
		}
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	port := cfg.Port
	if !isPortFmt(port) {
		port = "8080"
	}
	fmt.Printf("Guarda backend starting on :%s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func isPortFmt(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

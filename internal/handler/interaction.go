package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/guarda/backend/internal/repository"
	"github.com/guarda/backend/internal/service"
)

type InteractionHandler struct {
	repo    *repository.InteractionRepo
	checker *service.InteractionChecker
}

func NewInteractionHandler(repo *repository.InteractionRepo, checker *service.InteractionChecker) *InteractionHandler {
	return &InteractionHandler{repo: repo, checker: checker}
}

type checkRequest struct {
	ConversationID string   `json:"conversation_id" binding:"required"`
	Medications    []string `json:"medications" binding:"required"`
}

func (h *InteractionHandler) Check(c *gin.Context) {
	var req checkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	convID, err := uuid.Parse(req.ConversationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation_id"})
		return
	}

	ctx := c.Request.Context()
	var results []service.InteractionResult

	// Check all pairs
	for i := 0; i < len(req.Medications); i++ {
		for j := i + 1; j < len(req.Medications); j++ {
			result, err := h.checker.Check(ctx, req.Medications[i], req.Medications[j])
			if err != nil {
				continue
			}

			// Save to DB
			desc := result.Description
			rec := result.Recommendation
			interaction := &model.Interaction{
				ConversationID: convID,
				DrugA:          result.DrugA,
				DrugB:          result.DrugB,
				Severity:       result.Severity,
				Description:    &desc,
				Recommendation: &rec,
				Source:         result.Source,
			}
			h.repo.Create(ctx, interaction)

			results = append(results, *result)
		}
	}

	if results == nil {
		results = []service.InteractionResult{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

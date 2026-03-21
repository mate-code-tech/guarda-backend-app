package handler

import (
	"encoding/json"
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
	ConversationID string          `json:"conversation_id" binding:"required"`
	Medications    json.RawMessage `json:"medications" binding:"required"`
}

type medicationInput struct {
	InputName   string `json:"input_name"`
	GenericName string `json:"generic_name"`
}

func parseMedications(raw json.RawMessage) []medicationInput {
	// Try as array of objects first
	var meds []medicationInput
	if err := json.Unmarshal(raw, &meds); err == nil && len(meds) > 0 && meds[0].GenericName != "" {
		return meds
	}

	// Fallback: array of strings
	var names []string
	if err := json.Unmarshal(raw, &names); err == nil {
		var result []medicationInput
		for _, n := range names {
			result = append(result, medicationInput{InputName: n, GenericName: n})
		}
		return result
	}

	return nil
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

	meds := parseMedications(req.Medications)
	if len(meds) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "medications required"})
		return
	}

	// Build lookup from generic_name → input_name
	nameMap := make(map[string]string)
	for _, m := range meds {
		nameMap[m.GenericName] = m.InputName
	}

	ctx := c.Request.Context()
	var results []interactionResponse

	for i := 0; i < len(meds); i++ {
		for j := i + 1; j < len(meds); j++ {
			result, err := h.checker.Check(ctx, meds[i].GenericName, meds[j].GenericName)
			if err != nil {
				continue
			}

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

			results = append(results, interactionResponse{
				DrugA:          result.DrugA,
				DrugB:          result.DrugB,
				InputNameA:     nameMap[result.DrugA],
				InputNameB:     nameMap[result.DrugB],
				Severity:       result.Severity,
				Description:    result.Description,
				Recommendation: result.Recommendation,
				Source:         result.Source,
			})
		}
	}

	if results == nil {
		results = []interactionResponse{}
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

type interactionResponse struct {
	DrugA          string `json:"drug_a"`
	DrugB          string `json:"drug_b"`
	InputNameA     string `json:"input_name_a"`
	InputNameB     string `json:"input_name_b"`
	Severity       string `json:"severity"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
	Source         string `json:"source"`
}

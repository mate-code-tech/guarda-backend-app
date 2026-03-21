package service

import (
	"context"
)

type InteractionResult struct {
	DrugA          string `json:"drug_a"`
	DrugB          string `json:"drug_b"`
	Severity       string `json:"severity"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
	Source         string `json:"source"`
}

type InteractionChecker struct {
	dataset *InteractionDataset
	ai      *AIService
}

func NewInteractionChecker(dataset *InteractionDataset, ai *AIService) *InteractionChecker {
	return &InteractionChecker{dataset: dataset, ai: ai}
}

func (c *InteractionChecker) Check(ctx context.Context, drugA, drugB string) (*InteractionResult, error) {
	// Level 1: CSV dataset lookup
	if desc, ok := c.dataset.Lookup(drugA, drugB); ok {
		return &InteractionResult{
			DrugA:          drugA,
			DrugB:          drugB,
			Severity:       classifySeverityFromDesc(desc),
			Description:    desc,
			Recommendation: "Consulte con su médico antes de combinar estos medicamentos.",
			Source:         "dataset",
		}, nil
	}

	// Level 2: AI fallback
	if c.ai != nil {
		result, err := c.ai.CheckInteraction(ctx, drugA, drugB)
		if err == nil {
			result.Source = "ai_fallback"
			return result, nil
		}
	}

	return &InteractionResult{
		DrugA:          drugA,
		DrugB:          drugB,
		Severity:       "none",
		Description:    "No se encontraron interacciones conocidas entre estos medicamentos.",
		Recommendation: "Si tiene dudas, consulte con su médico.",
		Source:         "none",
	}, nil
}

func classifySeverityFromDesc(desc string) string {
	// Simple heuristic based on common keywords
	lower := removeAccents(desc)
	switch {
	case contains(lower, "serious", "severe", "fatal", "death", "contraindicated", "bleeding", "hemorrhag"):
		return "severe"
	case contains(lower, "moderate", "significant", "monitor", "caution"):
		return "moderate"
	case contains(lower, "mild", "minor", "low risk"):
		return "mild"
	default:
		return "moderate" // default to moderate for known interactions
	}
}

func contains(s string, keywords ...string) bool {
	for _, k := range keywords {
		if len(k) > 0 && len(s) >= len(k) {
			for i := 0; i <= len(s)-len(k); i++ {
				if s[i:i+len(k)] == k {
					return true
				}
			}
		}
	}
	return false
}

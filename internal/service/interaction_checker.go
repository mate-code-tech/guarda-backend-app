package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
)

type InteractionResult struct {
	DrugA          string `json:"drug_a"`
	DrugB          string `json:"drug_b"`
	Severity       string `json:"severity"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
	Source         string `json:"source"`
}

type ProfileContext struct {
	Name       string
	Age        int
	Conditions []string
	Allergies  []string
	IsForSelf  bool
}

type ProfileWarning struct {
	Type           string `json:"type"`
	Severity       string `json:"severity"`
	Drug           string `json:"drug"`
	Description    string `json:"description"`
	Recommendation string `json:"recommendation"`
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

func (c *InteractionChecker) CheckProfileWarnings(ctx context.Context, medications []string, profile *ProfileContext) []ProfileWarning {
	if profile == nil || c.ai == nil {
		return nil
	}

	// Build profile summary for the AI
	var profileParts []string
	if profile.Age > 0 {
		profileParts = append(profileParts, fmt.Sprintf("Edad: %d años", profile.Age))
	}
	if len(profile.Conditions) > 0 {
		profileParts = append(profileParts, fmt.Sprintf("Condiciones: %s", strings.Join(profile.Conditions, ", ")))
	}
	if len(profile.Allergies) > 0 {
		profileParts = append(profileParts, fmt.Sprintf("Alergias: %s", strings.Join(profile.Allergies, ", ")))
	}

	// No profile info to check against
	if len(profileParts) == 0 {
		return nil
	}

	profileSummary := strings.Join(profileParts, ". ")
	medsStr := strings.Join(medications, ", ")

	prompt := fmt.Sprintf(`Perfil del paciente: %s
Medicamentos: %s

Analizá si algún medicamento es riesgoso para este perfil. Considerá:
- Contraindicaciones por edad (niños, adultos mayores)
- Contraindicaciones por condiciones preexistentes (embarazo, diabetes, hipertensión, asma, etc.)
- Alergias reportadas y reactividad cruzada entre familias de medicamentos

SOLO JSON, sin markdown. Devolvé un array de alertas. Si no hay alertas, devolvé [].
Cada alerta:
{"type":"age|condition|allergy","severity":"mild|moderate|severe","drug":"nombre del medicamento","description":"explicación breve en español","recommendation":"recomendación breve en español"}

Sé preciso. NO inventes riesgos inexistentes. Solo alertá sobre contraindicaciones médicamente documentadas.`, profileSummary, medsStr)

	resp, err := c.ai.utilModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil
	}

	text := strings.TrimSpace(extractTextFromResp(resp))
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var warnings []ProfileWarning
	if err := json.Unmarshal([]byte(text), &warnings); err != nil {
		return nil
	}

	return warnings
}

func extractTextFromResp(resp *genai.GenerateContentResponse) string {
	var text string
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if t, ok := part.(genai.Text); ok {
				text += string(t)
			}
		}
	}
	return text
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

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"github.com/guarda/backend/internal/model"
	"google.golang.org/api/option"
)

const systemPrompt = `Sos Guarda, un asistente médico virtual argentino especializado en detectar interacciones medicamentosas peligrosas.

## REGLA MÁS IMPORTANTE
Cada vez que llames una función, SIEMPRE escribí un mensaje de texto ANTES de la llamada. NUNCA llames una función sin incluir texto. Esto es obligatorio sin excepción.

## Personalidad
- Hablás en español argentino natural (vos, sos, tomás, etc.)
- Sos cálido, empático pero directo. No usás jerga médica innecesaria.
- Respondés de forma concisa pero humana, como si hablaras con un amigo.

## Flujo de la conversación

### 1. Bienvenida (primer mensaje)
El usuario te escribe por primera vez. Respondé con texto:
"¡Hola! Soy Guarda, tu asistente para chequear interacciones entre medicamentos. ¿Cómo preferís que hablemos, por voz o por texto?"
NO llames ninguna función.

### 2. Selección de modo
El usuario elige voz o texto. Respondé con texto Y llamá a select_mode:
Texto: "¡Buenísimo! Vamos por [modo]. Contame, ¿qué medicamentos estás tomando?"
Función: select_mode con mode "voice" o "text"

### 3. El usuario menciona medicamentos
Respondé con texto Y llamá a normalize_medications:
Texto: "¡Perfecto! Dejame buscar esos medicamentos en mi base de datos..."
Función: normalize_medications con los nombres tal cual los escribió el usuario
NO llames a check_interactions todavía.

### 4. El usuario confirma los medicamentos
El usuario dice "sí", "correcto", "dale", etc. Respondé con texto Y llamá a check_interactions:
Texto: "¡Genial! Esperame un segundito mientras analizo si hay alguna interacción entre tus medicamentos..."
Función: check_interactions con medications vacío: {"medications": []}

## Reglas de funciones
- MÁXIMO UNA función por respuesta.
- select_mode: UNA SOLA VEZ en toda la conversación.
- NUNCA llames normalize_medications y check_interactions juntas.
- NUNCA llames la misma función dos veces con los mismos argumentos.
- NUNCA normalices nombres vos mismo. SIEMPRE usá normalize_medications.
- NUNCA inventes interacciones.
- Si normalize_medications devuelve generic_name vacío, pedile al usuario que revise ese nombre.

## Typos y nombres argentinos
Los usuarios escriben con typos y nombres coloquiales. NO les pidas que corrijan. Pasá todo a normalize_medications:
- "tafiro", "tafirol" → paracetamol
- "bayaspirina", "aspi" → aspirina
- "ibu", "ibuprofeno" → ibuprofeno
- "buscapina", "busca" → buscapina

## Qué NO hacer
- No diagnostiques enfermedades
- No recomiendes dosis ni medicamentos alternativos
- No inventes datos que no vengan de las funciones
- NUNCA mandes una función sin texto`

type AIService struct {
	client    *genai.Client
	model     *genai.GenerativeModel // chat model with system prompt
	utilModel *genai.GenerativeModel // utility model without system prompt
}

type AIResponse struct {
	Text          string
	FunctionCalls []FunctionCall
}

type FunctionCall struct {
	Name string                 `json:"name"`
	Args map[string]interface{} `json:"args"`
}

func NewAIService(ctx context.Context, apiKey string) (*AIService, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("creating Gemini client: %w", err)
	}

	model := client.GenerativeModel("gemini-3.1-flash-lite-preview")
	model.SystemInstruction = genai.NewUserContent(genai.Text(systemPrompt))
	model.Temperature = genai.Ptr[float32](0.3)

	utilModel := client.GenerativeModel("gemini-3.1-flash-lite-preview")
	utilModel.Temperature = genai.Ptr[float32](0.2)

	return &AIService{client: client, model: model, utilModel: utilModel}, nil
}

func (s *AIService) SetTools(tools []*genai.Tool) {
	s.model.Tools = tools
}

func (s *AIService) Chat(ctx context.Context, messages []model.Message) (*AIResponse, error) {
	cs := s.model.StartChat()

	// Build history from messages
	for _, msg := range messages[:len(messages)-1] {
		switch msg.Role {
		case "user":
			if msg.Content != nil {
				cs.History = append(cs.History, &genai.Content{
					Parts: []genai.Part{genai.Text(*msg.Content)},
					Role:  "user",
				})
			}
		case "assistant":
			if msg.Content != nil {
				cs.History = append(cs.History, &genai.Content{
					Parts: []genai.Part{genai.Text(*msg.Content)},
					Role:  "model",
				})
			}
		case "function":
			if msg.Content != nil && msg.ToolCallID != nil {
				cs.History = append(cs.History, &genai.Content{
					Parts: []genai.Part{genai.FunctionResponse{
						Name:     *msg.ToolCallID,
						Response: map[string]interface{}{"result": *msg.Content},
					}},
					Role: "function",
				})
			}
		}
	}

	// Send last message
	lastMsg := messages[len(messages)-1]
	var lastContent string
	if lastMsg.Content != nil {
		lastContent = *lastMsg.Content
	}

	resp, err := cs.SendMessage(ctx, genai.Text(lastContent))
	if err != nil {
		return nil, fmt.Errorf("sending message to Gemini: %w", err)
	}

	return parseGeminiResponse(resp), nil
}

func parseGeminiResponse(resp *genai.GenerateContentResponse) *AIResponse {
	result := &AIResponse{}

	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			switch v := part.(type) {
			case genai.Text:
				result.Text += string(v)
			case genai.FunctionCall:
				result.FunctionCalls = append(result.FunctionCalls, FunctionCall{
					Name: v.Name,
					Args: v.Args,
				})
			}
		}
	}

	return result
}

func (s *AIService) NormalizeMedication(ctx context.Context, name string) (string, error) {
	prompt := fmt.Sprintf(`Medicamento: "%s"
Devolvé SOLO el nombre genérico INN en inglés, minúsculas, una sola palabra o dos máximo.
Ejemplos: tafirol→acetaminophen, aspirina→aspirin, ibuprofeno→ibuprofen, buscapina→hyoscine, omeprazol→omeprazole, amoxidal→amoxicillin
Si no lo reconocés: UNKNOWN`, name)

	resp, err := s.utilModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", err
	}

	text := strings.ToLower(strings.TrimSpace(extractText(resp)))
	if text == "" || text == "unknown" {
		return "", fmt.Errorf("AI could not identify medication: %s", name)
	}
	return text, nil
}

func (s *AIService) CheckInteraction(ctx context.Context, drugA, drugB string) (*InteractionResult, error) {
	prompt := fmt.Sprintf(`Interacción entre "%s" y "%s".
SOLO JSON, sin markdown:
{"severity":"none|mild|moderate|severe","description":"máximo 10 palabras en español","recommendation":"máximo 10 palabras en español"}
Si no hay interacción real, severity es "none". No inventes riesgos que no existen. Sé preciso y breve.`, drugA, drugB)

	resp, err := s.utilModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, err
	}

	text := extractText(resp)
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)

	var result struct {
		Severity       string `json:"severity"`
		Description    string `json:"description"`
		Recommendation string `json:"recommendation"`
	}
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return &InteractionResult{
			DrugA:          drugA,
			DrugB:          drugB,
			Severity:       "moderate",
			Description:    text,
			Recommendation: "Consulte con su médico.",
		}, nil
	}

	return &InteractionResult{
		DrugA:          drugA,
		DrugB:          drugB,
		Severity:       result.Severity,
		Description:    result.Description,
		Recommendation: result.Recommendation,
	}, nil
}

func (s *AIService) Close() {
	s.client.Close()
}

func extractText(resp *genai.GenerateContentResponse) string {
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

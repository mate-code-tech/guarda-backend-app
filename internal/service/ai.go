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

## Personalidad
- Hablás en español argentino natural (vos, sos, tomás, etc.)
- Sos cálido, empático pero directo. No usás jerga médica innecesaria.
- Respondés de forma concisa.

## Flujo inicial (PRIMER MENSAJE)
Cuando el usuario envía su primer mensaje en una conversación nueva:
- Saludalo y presentate brevemente como Guarda.
- Preguntale cómo prefiere interactuar: por voz o por texto.
- Respondé SOLO con texto, NO llames ninguna función todavía.

Cuando el usuario responde eligiendo un modo (ej: "voz", "por voz", "hablando", "texto", "escribiendo", "teclado"):
- Llamá a select_mode con el modo elegido: "voice" si eligió voz, "text" si eligió texto.
- Podés incluir un mensaje breve confirmando el modo.

## Flujo de medicamentos (MUY IMPORTANTE)
El flujo tiene DOS pasos separados controlados por el usuario:

### Paso 1: El usuario menciona medicamentos
Cuando el usuario menciona medicamentos (marcas, genéricos, con typos, en español o inglés):
- Llamá SOLO a normalize_medications con los nombres tal cual los escribió el usuario.
- NO llames a check_interactions todavía.
- El frontend le va a mostrar al usuario el listado de medicamentos normalizados para que confirme.

### Paso 2: El usuario confirma que los datos son correctos
Cuando el usuario dice algo como "sí", "son correctos", "confirmo", "dale", "ok chequeá":
- Llamá a check_interactions con un array vacío en medications: {"medications": []}
- El frontend ya tiene los medicamentos del paso anterior y se encarga de enviarlos al endpoint.

## Reglas críticas
- MÁXIMO UNA función por respuesta. NUNCA llames dos o más funciones en la misma respuesta.
- select_mode se llama UNA SOLA VEZ en toda la conversación (cuando el usuario elige modo). Después NUNCA más.
- NUNCA llames a la misma función dos veces con los mismos argumentos.
- NUNCA intentes normalizar nombres vos mismo. SIEMPRE usá normalize_medications.
- NUNCA inventes interacciones. Solo reportá lo que devuelve check_interactions.
- Si normalize_medications devuelve un generic_name vacío, pedile al usuario que revise ese nombre.

## Manejo de typos y nombres informales
Los usuarios argentinos van a escribir con typos, abreviaciones y nombres coloquiales. Ejemplos:
- "tafiro", "tafirol", "tafi" → Tafirol (paracetamol/acetaminophen)
- "bayaspirina", "aspirina", "aspi" → Aspirina (aspirin)
- "ibu", "ibuprofeno" → Ibuprofeno (ibuprofen)
- "buscapina", "busca" → Buscapina (hyoscine)
NO le pidas al usuario que corrija el nombre. Pasalo a normalize_medications que maneja typos.

## Conversación general
Si el usuario conversa sin mencionar medicamentos ni elegir modo, respondé normalmente con texto.

## Qué NO hacer
- No diagnostiques enfermedades
- No recomiendes dosis
- No sugieras medicamentos alternativos
- No des consejos de tratamiento
- No inventes datos que no vengan de las funciones`

type AIService struct {
	client *genai.Client
	model  *genai.GenerativeModel
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

	return &AIService{client: client, model: model}, nil
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
	prompt := fmt.Sprintf(`Sos un experto farmacéutico. El usuario escribió "%s" refiriéndose a un medicamento.
Puede tener typos, estar en español, ser una marca comercial argentina, o una abreviación informal.

Ejemplos de resolución:
- "tafiro" o "tafirol" → acetaminophen
- "bayaspirina" o "aspi" → aspirin
- "ibu" o "ibuprofeno" → ibuprofen
- "buscapina" → hyoscine
- "amoxidal" → amoxicillin
- "rivotril" → clonazepam

Respondé SOLAMENTE con el nombre genérico INN en inglés, en minúsculas, sin puntuación ni texto extra.
Si no podés identificar el medicamento con certeza, respondé exactamente: UNKNOWN`, name)

	resp, err := s.model.GenerateContent(ctx, genai.Text(prompt))
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
	prompt := fmt.Sprintf(`Analizá la interacción entre "%s" y "%s".
Respondé SOLAMENTE con un JSON válido con este formato exacto (sin markdown ni texto extra):
{"severity":"none|mild|moderate|severe","description":"descripción breve en español","recommendation":"recomendación en español"}`, drugA, drugB)

	resp, err := s.model.GenerateContent(ctx, genai.Text(prompt))
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

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

const systemPrompt = `Sos Guarda, un asistente argentino que chequea interacciones entre medicamentos.

## REGLAS OBLIGATORIAS
1. SIEMPRE escribí texto ANTES de llamar una función. NUNCA llames una función sin texto. El texto debe ser conversacional e invitar al usuario a seguir (nunca solo "perfecto" o "anotado").
2. MÁXIMO UNA función por respuesta.
3. Hablás en argentino natural (vos, sos, tomás). Sos cálido y directo, como un amigo.
4. Respuestas CORTAS. Máximo 2 oraciones.
5. Siempre que guardes datos del usuario, respondé reconociendo lo que dijo Y hacé la siguiente pregunta del flujo. Ejemplo: "Buenísimo, lo tengo en cuenta. ¿Qué medicamentos estás tomando?"

## FLUJO (seguilo en orden estricto)

PASO 1 — BIENVENIDA
Respondé: "¡Hola! Soy Guarda, tu asistente para chequear interacciones entre medicamentos. ¿Cómo te llamás?"
No llames ninguna función.

PASO 2 — NOMBRE RECIBIDO
Guardá el nombre y preguntá edad + condiciones/alergias en UNA sola pregunta.
Texto: "¡Hola [nombre]! ¿Cuántos años tenés? ¿Y tenés alguna condición médica o alergia?"
Función: save_guest_profile con name, is_for_self=true

IMPORTANTE: Si en este mensaje el usuario TAMBIÉN menciona medicamentos (ej: "Soy Leo, tomo tafirol"), NO vuelvas a preguntar por medicamentos. Guardá el nombre Y normalizá los medicamentos en el siguiente turno.

PASO 3 — EDAD Y CONDICIONES RECIBIDAS → PEDIR MEDICAMENTOS
Guardá lo que dijo y pedí medicamentos.
Texto: "Dale [nombre], ¿qué medicamentos estás tomando?"
Función: save_guest_profile con age, conditions, allergies

IMPORTANTE: Si el usuario ya mencionó medicamentos antes, NO vuelvas a preguntar. Pasá directo a normalizar con normalize_medications.

PASO 4 — MEDICAMENTOS RECIBIDOS → NORMALIZAR
Texto: "Dejame buscar esos medicamentos... ¿Son estos los que tomás? ¿Querés agregar alguno más?"
Función: normalize_medications con los nombres tal cual los dijo el usuario
NUNCA pidas medicamentos de nuevo si ya los tenés.

PASO 5 — CONFIRMAR MEDICAMENTOS
El frontend muestra los medicamentos normalizados en pantalla. El usuario confirma ("sí", "dale", "ok", "no, esos nomas") o pide agregar más.
Si confirma:
Texto: "¡Genial! Analizando interacciones..."
Función: check_interactions con {"medications": []}

Si quiere agregar más, pedile cuáles y volvé al paso 4.

PASO 6 — DESPUÉS DE LOS RESULTADOS
Después de que check_interactions devuelve resultados, el frontend los muestra en pantalla. Respondé con un BREVE resumen de lo encontrado y preguntá si tiene otra duda.
Si hay interacciones SEVERAS o MODERADAS, empezá tu respuesta con "¡Guarda!" como expresión de advertencia/cuidado. Ejemplo: "¡Guarda! Encontré una interacción importante entre X y Y. Consultá con tu médico antes de combinarlos. ¿Tenés alguna otra duda?"
Si NO hay interacciones peligrosas (todas none o mild), respondé normalmente sin "¡Guarda!". Ejemplo: "Todo tranqui, no encontré interacciones preocupantes. ¿Tenés alguna otra duda o querés chequear otros medicamentos?"
NO llames ninguna función.

Si el usuario quiere chequear otros medicamentos, volvé al paso 4 (pedir medicamentos y normalizar).
Si el usuario tiene una duda general sobre un medicamento, respondé brevemente con lo que sabés (sin inventar).
Si dice que no, despedite brevemente Y llamá a end_conversation:
Texto: "¡Cuidate [nombre]! Siempre consultá con tu médico. ¡Hasta la próxima!"
Función: end_conversation

## REGLA CRÍTICA: NUNCA REPITAS PREGUNTAS
- Si el usuario ya dijo medicamentos EN CUALQUIER MOMENTO de la conversación, NUNCA vuelvas a preguntar "¿qué medicamentos tomás?". Usá lo que ya te dijo.
- Si el usuario da nombre + edad + medicamentos todo junto, guardá el perfil Y normalizá los medicamentos sin hacer preguntas intermedias.
- Si el usuario quiere saltear preguntas e ir directo a medicamentos, dejalo.

## FUNCIONES
- save_guest_profile: guarda datos del usuario. Incluí solo campos nuevos.
- normalize_medications: convierte nombres de medicamentos a genéricos. Pasá los nombres tal cual.
- check_interactions: señal para chequear interacciones. Siempre con medications vacío.
- end_conversation: señal para terminar la conversación. Llamar cuando el usuario se despide o dice que no tiene más dudas.

## TYPOS ARGENTINOS
No pidas que corrijan. Pasá todo a normalize_medications:
"tafiro"/"tafirol" → paracetamol, "aspi"/"bayaspirina" → aspirina, "ibu" → ibuprofeno, "busca"/"buscapina" → buscapina

## PROHIBIDO
- Diagnosticar enfermedades
- Recomendar dosis o medicamentos
- Inventar datos
- Preguntar si es para vos o para otra persona
- Decir "ya lo anoté" o similares
- Llamar función sin texto`

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

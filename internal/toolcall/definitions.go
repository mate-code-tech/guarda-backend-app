package toolcall

import (
	"github.com/google/generative-ai-go/genai"
)

func GetToolDefinitions() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "save_guest_profile",
					Description: "Guarda la información personal del usuario recopilada durante el onboarding. Se llama cada vez que el usuario brinda datos personales como nombre, edad, condiciones, alergias, motivo de consulta o si es para él/ella u otra persona.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"name": {
								Type:        genai.TypeString,
								Description: "Nombre del usuario",
							},
							"age": {
								Type:        genai.TypeInteger,
								Description: "Edad del usuario en años",
							},
							"conditions": {
								Type:        genai.TypeArray,
								Description: "Condiciones preexistentes (embarazo, diabetes, hipertensión, etc.) en español",
								Items:       &genai.Schema{Type: genai.TypeString},
							},
							"allergies": {
								Type:        genai.TypeArray,
								Description: "Alergias conocidas a medicamentos en español",
								Items:       &genai.Schema{Type: genai.TypeString},
							},
							"consultation_reason": {
								Type:        genai.TypeString,
								Description: "Motivo de la consulta (dolor de cabeza, gripe, etc.)",
							},
							"is_for_self": {
								Type:        genai.TypeBoolean,
								Description: "true si la consulta es para el propio usuario, false si es para otra persona",
							},
						},
					},
				},
				{
					Name:        "normalize_medications",
					Description: "Normaliza nombres de medicamentos (marcas comerciales, nombres en español, con typos) a sus nombres genéricos INN en inglés",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"medications": {
								Type:        genai.TypeArray,
								Description: "Lista de nombres de medicamentos tal cual los escribió el usuario",
								Items:       &genai.Schema{Type: genai.TypeString},
							},
						},
						Required: []string{"medications"},
					},
				},
				{
					Name:        "check_interactions",
					Description: "Señal para que el frontend ejecute el chequeo de interacciones medicamentosas. Se llama cuando el usuario confirma que los medicamentos normalizados son correctos.",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"medications": {
								Type:        genai.TypeArray,
								Description: "Array vacío, el frontend ya tiene los medicamentos",
								Items:       &genai.Schema{Type: genai.TypeString},
							},
						},
						Required: []string{"medications"},
					},
				},
				{
					Name:        "end_conversation",
					Description: "Señal para que el frontend termine la conversación y muestre la pantalla de despedida con botón para volver al inicio.",
					Parameters: &genai.Schema{
						Type:       genai.TypeObject,
						Properties: map[string]*genai.Schema{},
					},
				},
			},
		},
	}
}

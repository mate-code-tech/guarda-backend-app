package toolcall

import (
	"github.com/google/generative-ai-go/genai"
)

func GetToolDefinitions() []*genai.Tool {
	return []*genai.Tool{
		{
			FunctionDeclarations: []*genai.FunctionDeclaration{
				{
					Name:        "select_mode",
					Description: "Señala al frontend qué modo de interacción eligió el usuario",
					Parameters: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"mode": {
								Type:        genai.TypeString,
								Description: "Modo elegido: 'voice' para voz o 'text' para texto",
								Enum:        []string{"voice", "text"},
							},
						},
						Required: []string{"mode"},
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
			},
		},
	}
}

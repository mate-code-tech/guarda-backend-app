# Plan Backend — Guarda (`guarda-backend-app`)

## Context

Hackathon biotech/health. App mobile-first llamada **Guarda** con un asistente conversacional (orbe animado) que detecta riesgos en combinaciones de medicamentos y cruza medicamentos con análisis clínicos. Backend en Go. Sin login, solo guest_id.

**Nota:** El backend nunca recibe audio/voz. Toda la transcripción speech-to-text ocurre en el frontend (Web Speech API). El backend solo recibe texto plano.

---

## Setup

Go 1.22+, `go mod init github.com/guarda/backend`.
Deps: `gin-gonic/gin`, `jackc/pgx/v5`, `google/uuid`, `google/generative-ai-go` (Gemini SDK), `joho/godotenv`.

---

## Estructura de directorios

```
cmd/server/main.go                  # Entry point
internal/
├── config/config.go                # Config desde env vars
├── middleware/
│   ├── cors.go                     # CORS para frontend
│   └── guest.go                    # Extraer X-Guest-ID, validar
├── handler/
│   ├── guest.go                    # POST /guests
│   ├── chat.go                     # POST /chat/message (JSON)
│   ├── medication.go               # POST /medications/validate, /confirm
│   ├── interaction.go              # POST /interactions/check
│   └── upload.go                   # POST /upload/lab
├── service/
│   ├── ai.go                       # Gemini chat + function calling
│   ├── normalizer.go               # Normalización: diccionario → fuzzy → AI
│   ├── interaction_checker.go      # PubChem → dataset → AI fallback
│   ├── pubchem.go                  # Cliente PubChem API
│   ├── lab_parser.go               # OCR/Vision para análisis
│   └── dataset.go                  # Carga CSV datasets argentinos
├── model/                          # Guest, Conversation, Message, Medication, Interaction
├── repository/                     # CRUD para cada modelo
├── toolcall/
│   ├── definitions.go              # Schemas de tools para Gemini
│   ├── executor.go                 # Despachar tool_call → handler
│   └── handlers.go                 # Handlers individuales
└── db/
    ├── postgres.go                 # Connection pool
    └── migrations/                 # 001-005 SQL files
data/drug_dictionary.csv            # Diccionario local de medicamentos
.env                                # DB_URL, GEMINI_API_KEY, PORT
Makefile
```

---

## Endpoints

| Método | Ruta                             | Descripción                        |
| ------ | -------------------------------- | ---------------------------------- |
| POST   | `/api/v1/guests`                 | Registrar guest                    |
| POST   | `/api/v1/chat/message`           | Enviar mensaje, respuesta JSON       |
| POST   | `/api/v1/medications/validate`   | Normalizar nombres de medicamentos |
| POST   | `/api/v1/medications/confirm`    | Confirmar medicamentos validados   |
| POST   | `/api/v1/interactions/check`     | Ejecutar cruce de interacciones    |
| POST   | `/api/v1/upload/lab`             | Subir análisis clínico             |
| GET    | `/api/v1/conversations/:id`      | Historial de conversación          |

Todos requieren header `X-Guest-ID` (excepto `POST /guests`).

---

## Schema PostgreSQL

```sql
-- guests
CREATE TABLE guests (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    preferred_mode TEXT NOT NULL DEFAULT 'text'
);

-- conversations
CREATE TABLE conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    guest_id UUID NOT NULL REFERENCES guests(id),
    flow_type TEXT NOT NULL DEFAULT 'general',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- messages
CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    role TEXT NOT NULL,
    content TEXT,
    tool_calls JSONB,
    tool_call_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- medications
CREATE TABLE medications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    input_name TEXT NOT NULL,
    generic_name TEXT NOT NULL,
    confirmed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- interactions
CREATE TABLE interactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    drug_a TEXT NOT NULL,
    drug_b TEXT NOT NULL,
    severity TEXT NOT NULL,
    description TEXT,
    recommendation TEXT,
    source TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- uploads
CREATE TABLE uploads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    file_path TEXT NOT NULL,
    extracted_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

---

## Flujo de Chat (core)

1. Handler recibe mensaje → guarda en DB
2. Carga historial completo de conversación
3. Envía a Gemini con function declarations
4. Si hay function calls → ejecutar via executor → guardar resultado como function response → volver a paso 3
5. Si es mensaje normal → construir JSON response con message + tool_calls
6. Guardar mensaje del asistente en DB
7. Devolver JSON al cliente (HTTP 200)
8. Cap de **5 rondas** de tool-calls para evitar loops

---

## Normalización de Medicamentos (3 niveles)

El objetivo es resolver cualquier input del usuario (marca comercial, nombre en español, typo) a un **nombre genérico PubChem-compatible** (INN en inglés: `ibuprofen`, `aspirin`, `acetaminophen`, etc.) para poder consultarlo en la API de PubChem.

1. **Diccionario** (rápido): CSV en memoria con mapeo marca/español → nombre PubChem, match case-insensitive sin acentos
2. **Fuzzy match**: Levenshtein ≤ 2 contra diccionario → mismo mapeo a nombre PubChem
3. **AI fallback**: Prompt al LLM pidiendo explícitamente el INN name en inglés que se usaría en PubChem

---

## Checker de Interacciones (3 niveles)

1. **PubChem**: Resolver CID por nombre, consultar propiedades
2. **Dataset argentino**: CSV con interacciones conocidas
3. **AI fallback**: LLM clasifica severidad (`none`/`mild`/`moderate`/`severe`) → marcar `source: "ai_fallback"` + disclaimer

---

## Contrato de Integración con Frontend

### Headers

- `X-Guest-ID: <uuid>` en toda request (excepto `POST /guests`)

### Endpoints — Request / Response completos

---

#### `POST /api/v1/guests` — Registrar guest

**Request:**
```json
{ "preferred_mode": "text" }
```

**Response:** `201`
```json
{
  "id": "uuid",
  "created_at": "2026-03-21T...",
  "preferred_mode": "text"
}
```

---

#### `POST /api/v1/chat/message` — Enviar mensaje, respuesta JSON

**Request:**
```json
{ "conversation_id": "uuid | null", "message": "Tomo ibuprofeno y aspirina" }
```

**Response:** `200`
```json
{
  "conversation_id": "uuid",
  "message": "Veo que mencionás dos medicamentos...",
  "tool_calls": [
    {
      "name": "normalize_medications",
      "data": {
        "medications": [
          { "input_name": "ibuprofeno", "generic_name": "ibuprofen" },
          { "input_name": "aspirina", "generic_name": "aspirin" }
        ]
      }
    },
    {
      "name": "check_interactions",
      "data": {
        "results": [
          {
            "drug_a": "ibuprofen",
            "drug_b": "aspirin",
            "severity": "severe",
            "description": "Aumenta riesgo de sangrado GI",
            "recommendation": "No combinar sin supervisión médica",
            "source": "dataset"
          }
        ]
      }
    }
  ]
}
```

`tool_calls` es un array (puede estar vacío). El backend resuelve todos los tool-calls internamente (loop Gemini) antes de responder.

---

#### `POST /api/v1/medications/validate` — Normalizar nombres

**Request:**
```json
{
  "conversation_id": "uuid",
  "medications": ["ibuprofeno", "Tafirol", "bayaspirina"]
}
```

**Response:** `200`
```json
{
  "medications": [
    { "input_name": "ibuprofeno", "generic_name": "ibuprofen" },
    { "input_name": "Tafirol", "generic_name": "acetaminophen" },
    { "input_name": "bayaspirina", "generic_name": "aspirin" }
  ]
}
```

---

#### `POST /api/v1/medications/confirm` — Confirmar medicamentos

**Request:**
```json
{
  "conversation_id": "uuid",
  "medications": [
    { "generic_name": "ibuprofen" },
    { "generic_name": "acetaminophen" },
    { "generic_name": "aspirin" }
  ]
}
```

**Response:** `200`
```json
{ "confirmed": true, "count": 3 }
```

---

#### `POST /api/v1/interactions/check` — Cruce de interacciones

**Request:**
```json
{
  "conversation_id": "uuid",
  "medications": ["ibuprofen", "aspirin", "acetaminophen"]
}
```

**Response:** `200`
```json
{
  "results": [
    {
      "drug_a": "ibuprofen",
      "drug_b": "aspirin",
      "severity": "severe",
      "description": "Aumenta riesgo de sangrado gastrointestinal",
      "recommendation": "No combinar sin supervisión médica",
      "source": "dataset"
    },
    {
      "drug_a": "ibuprofen",
      "drug_b": "acetaminophen",
      "severity": "mild",
      "description": "Combinación generalmente segura en dosis terapéuticas",
      "recommendation": "Monitorear dosis total diaria",
      "source": "ai_fallback"
    }
  ]
}
```

Severidades posibles: `none`, `mild`, `moderate`, `severe`.

---

#### `POST /api/v1/upload/lab` — Subir análisis clínico

**Request:** `multipart/form-data`
```
file: <archivo PDF/imagen>
conversation_id: "uuid"
```

**Response:** `200`
```json
{
  "id": "uuid",
  "filename": "analisis_sangre.pdf",
  "extracted_data": {
    "hemoglobina": { "value": 14.2, "unit": "g/dL", "reference": "12-16" },
    "glucemia": { "value": 110, "unit": "mg/dL", "reference": "70-100" },
    "creatinina": { "value": 1.1, "unit": "mg/dL", "reference": "0.7-1.3" }
  }
}
```

---

#### `GET /api/v1/conversations/:id` — Historial de conversación

**Request:** solo header `X-Guest-ID`

**Response:** `200`
```json
{
  "id": "uuid",
  "guest_id": "uuid",
  "flow_type": "general",
  "status": "active",
  "created_at": "...",
  "messages": [
    { "id": "uuid", "role": "user", "content": "Tomo ibuprofeno", "created_at": "..." },
    { "id": "uuid", "role": "assistant", "content": "Veo que...", "tool_calls": [...], "created_at": "..." }
  ],
  "medications": [...],
  "interactions": [...]
}
```

---

## Tareas (ordenadas)

| #   | Tarea                                                            | Entregable                           |
| --- | ---------------------------------------------------------------- | ------------------------------------ |
| B1  | Project init: go mod, deps, estructura, Makefile                 | Proyecto compilable                  |
| B2  | Config + DB: config.go, postgres.go, pool                        | DB conectada                         |
| B3  | Migraciones SQL                                                  | Schema creado en PostgreSQL          |
| B4  | Guest handler + middleware                                       | Registro y auth de guests            |
| B5  | Models + repositories: structs y CRUD                            | Capa de datos completa               |
| B6  | Chat handler: JSON response (echo para testing)                  | Endpoint chat funcional              |
| B7  | AI service: Gemini SDK + function calling                        | IA responde mensajes                 |
| B8  | Function-call framework: declarations, executor, routing         | Function calls despachados           |
| B9  | Normalización: diccionario, fuzzy, AI fallback                   | Tool normalize_medications funcional |
| B10 | Medication handlers: validate + confirm                          | Flujo de validación completo         |
| B11 | Interaction checker: PubChem, dataset, AI fallback               | Tool check_interactions funcional    |
| B12 | File upload: multipart, storage, AI vision                       | Upload + extracción funcional        |
| B13 | Testing E2E, error handling, CORS                                | Backend integrado                    |

### Paralelización

- B5 y B6 en paralelo
- B9 y B11 son servicios independientes
- B12 totalmente independiente

### Orden sugerido para demo rápida

1. Scaffolding (B1-B3)
2. Guest system (B4)
3. Chat E2E con AI real (B5-B7) ← **primer milestone**
4. Tool-calls + normalización (B8-B10)
5. Interaction checking (B11)
6. File upload (B12)
7. Polish y testing (B13)


---

## Riesgos y Mitigaciones

| Riesgo                                       | Mitigación                                                                  |
| -------------------------------------------- | --------------------------------------------------------------------------- |
| PubChem sin endpoint directo de interacciones | Usar para validar nombres, interacciones via dataset + AI                   |
| Rate limits de Gemini                        | Timeout 30s, retry exponencial, cachear normalizaciones                     |
| Datasets argentinos incompletos              | Diccionario manual de 50+ marcas comunes como mínimo                        |
| Loop de tool-calls                           | Cap en 5 rondas por mensaje                                                |

---

## Verificación

- [ ] Guest se crea y persiste con UUID
- [ ] Chat E2E: recibir mensaje → respuesta JSON completa con message + tool_calls
- [ ] Decir medicamentos → normalización 3 niveles → confirmar → guardar en DB
- [ ] Cruce de interacciones → semáforo de severidad
- [ ] Subir análisis → extracción via AI vision → cruce con medicamentos
- [ ] CORS configurado correctamente para frontend
- [ ] Error handling consistente en todos los endpoints

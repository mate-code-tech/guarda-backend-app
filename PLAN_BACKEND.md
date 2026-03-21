# Plan Backend — Guarda (`guarda-backend-app`)

## Context

Hackathon biotech/health. App mobile-first llamada **Guarda** con un asistente conversacional (orbe animado) que detecta riesgos en combinaciones de medicamentos. Backend en Go. Sin login, solo guest_id.

**Nota:** El backend nunca recibe audio/voz. Toda la transcripción speech-to-text ocurre en el frontend (Web Speech API). El backend solo recibe texto plano.

---

## Setup

Go 1.22+, `go mod init github.com/guarda/backend`.
Deps: `gin-gonic/gin`, `jackc/pgx/v5`, `google/uuid`, `google/generative-ai-go` (Gemini SDK), `joho/godotenv`.

```bash
cp .env.example .env          # editar con GEMINI_API_KEY
docker compose up -d           # PostgreSQL 16
make run                       # servidor en :8080
```

---

## Estructura de directorios

```
cmd/server/main.go                  # Entry point
internal/
├── config/config.go                # Config desde env vars
├── middleware/
│   ├── cors.go                     # CORS permisivo para dev
│   └── guest.go                    # Extraer X-Guest-ID, validar
├── handler/
│   ├── guest.go                    # POST /guests
│   ├── chat.go                     # POST /chat/message (JSON)
│   └── interaction.go              # POST /interactions/check
├── service/
│   ├── ai.go                       # Gemini chat + function calling + system prompt
│   ├── normalizer.go               # Normalización: diccionario → RxNorm API → AI
│   ├── interaction_checker.go      # CSV dataset → AI fallback
│   ├── rxnorm.go                   # Cliente RxNorm API (normalización de nombres)
│   └── dataset.go                  # Carga CSV: drug_dictionary + interactions (Kaggle DDI)
├── model/                          # Guest, Conversation, Message, Interaction
├── repository/                     # CRUD para cada modelo
├── toolcall/
│   ├── definitions.go              # Schemas de tools para Gemini (select_mode, normalize, check)
│   └── executor.go                 # Despachar tool_call → handler o señal frontend
└── db/
    ├── postgres.go                 # Connection pool + migration runner
    └── migrations/001_create_tables.sql
data/drug_dictionary.csv            # Diccionario marcas argentinas → INN (~50 entries)
data/drug_interactions.csv          # Kaggle DDI dataset (usuario lo provee)
.env                                # DB_URL, GEMINI_API_KEY, PORT
Makefile
docker-compose.yml
```

---

## Endpoints

| Método | Ruta                           | Auth        | Descripción                     |
| ------ | ------------------------------ | ----------- | ------------------------------- |
| POST   | `/api/v1/guests`               | No          | Registrar guest                 |
| POST   | `/api/v1/chat/message`         | X-Guest-ID  | Enviar mensaje, respuesta JSON  |
| POST   | `/api/v1/interactions/check`   | X-Guest-ID  | Cruce de interacciones          |
| GET    | `/health`                      | No          | Health check                    |

---

## Schema PostgreSQL

```sql
CREATE TABLE IF NOT EXISTS guests (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    preferred_mode TEXT NOT NULL DEFAULT 'text'
);

CREATE TABLE IF NOT EXISTS conversations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    guest_id UUID NOT NULL REFERENCES guests(id),
    flow_type TEXT NOT NULL DEFAULT 'general',
    status TEXT NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id),
    role TEXT NOT NULL,
    content TEXT,
    tool_calls JSONB,
    tool_call_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS interactions (
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
```

---

## Flujo conversacional completo

El chat es AI-driven. Gemini controla el flujo via tool_calls que actúan como **señales para el frontend**. Solo `normalize_medications` se ejecuta en el backend.

### Paso 1: Bienvenida
```
User: "hola"
→ AI responde con texto: "¡Hola! Soy Guarda... ¿Cómo preferís interactuar, por voz o por texto?"
→ tool_calls: []
```

### Paso 2: Selección de modo
```
User: "por voz"
→ AI responde con mensaje breve
→ tool_calls: [{ "name": "select_mode", "data": { "mode": "voice" } }]
Frontend: activa modo voz
```

### Paso 3: Menciona medicamentos
```
User: "tomo tafirol y buscapina"
→ Backend ejecuta normalize_medications (diccionario → RxNorm → AI)
→ tool_calls: [{ "name": "normalize_medications", "data": { "medications": [...] } }]
→ message: texto del AI explicando lo que encontró
Frontend: muestra lista de medicamentos normalizados para confirmar
```

### Paso 4: Confirma medicamentos
```
User: "sí, están bien"
→ AI llama check_interactions como señal (NO se ejecuta en backend)
→ tool_calls: [{ "name": "check_interactions", "data": null }]
→ message: ""
Frontend: usa los meds que ya tiene → pega a POST /interactions/check
```

### Reglas del AI
- **Máximo UNA función por respuesta**
- `select_mode` se llama UNA SOLA VEZ en toda la conversación
- NUNCA llama `normalize_medications` y `check_interactions` juntas
- Cap de **5 rondas** de tool-calls para evitar loops

---

## Tool calls (Gemini function declarations)

| Tool                    | Ejecuta en backend | Descripción                                         |
| ----------------------- | ------------------ | --------------------------------------------------- |
| `select_mode`           | No (señal)         | Indica al frontend el modo elegido (voice/text)     |
| `normalize_medications` | Sí                 | Normaliza nombres a INN via diccionario/RxNorm/AI   |
| `check_interactions`    | No (señal)         | Indica al frontend que ejecute el chequeo           |

---

## Normalización de Medicamentos (3 niveles)

El objetivo es resolver cualquier input (marca, español, typo) a un **nombre genérico INN** en inglés.

1. **Diccionario local + fuzzy** (rápido): CSV en memoria con mapeo marca argentina → INN. Match case-insensitive sin acentos. Fuzzy matching con Levenshtein (distancia ≤ 2) y prefix match. ~50+ marcas argentinas.
2. **RxNorm API** (fallback): `GET https://rxnav.nlm.nih.gov/REST/rxcui.json?name={name}&search=2`. Sin API key, gratis, 20 req/s. Si no match exacto → approximate search.
3. **AI fallback**: Prompt a Gemini con ejemplos de typos argentinos. Devuelve "UNKNOWN" si no puede identificar.

---

## Checker de Interacciones (2 niveles)

> **NOTA:** La API de interacciones de NLM fue discontinuada en enero 2024.

1. **Dataset CSV local**: Kaggle DDI (DrugBank v5.1). Formato: `Drug 1, Drug 2, Interaction Description`. Búsqueda bidireccional case-insensitive por nombre INN. Clasificación de severidad por heurística de keywords.
2. **Gemini AI fallback**: Clasifica severidad (`none`/`mild`/`moderate`/`severe`), describe interacción y da recomendación. Source: `"ai_fallback"`.

---

## Contrato de Integración con Frontend

### Headers

- `X-Guest-ID: <uuid>` en toda request (excepto `POST /guests`)

### Endpoints — Request / Response

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

#### `POST /api/v1/chat/message` — Enviar mensaje

**Request:**
```json
{ "conversation_id": "uuid | null", "message": "texto del usuario" }
```

`conversation_id` es `null` en el primer mensaje. El backend crea la conversación y devuelve el ID.

**Response (con texto):** `200`
```json
{
  "conversation_id": "uuid",
  "message": "¡Hola! Soy Guarda...",
  "tool_calls": []
}
```

**Response (con select_mode):** `200`
```json
{
  "conversation_id": "uuid",
  "message": "¡Dale, vamos por voz!",
  "tool_calls": [{ "name": "select_mode", "data": { "mode": "voice" } }]
}
```

**Response (con normalize_medications):** `200`
```json
{
  "conversation_id": "uuid",
  "message": "Encontré estos medicamentos...",
  "tool_calls": [
    {
      "name": "normalize_medications",
      "data": {
        "medications": [
          { "input_name": "tafirol", "generic_name": "acetaminophen" },
          { "input_name": "buscapina", "generic_name": "hyoscine" }
        ]
      }
    }
  ]
}
```

**Response (con check_interactions — señal):** `200`
```json
{
  "conversation_id": "uuid",
  "message": "",
  "tool_calls": [{ "name": "check_interactions", "data": null }]
}
```

El frontend recibe `check_interactions` → usa los meds que ya tiene del paso anterior → pega a `POST /interactions/check`.

---

#### `POST /api/v1/interactions/check` — Cruce de interacciones

**Request:**
```json
{
  "conversation_id": "uuid",
  "medications": ["acetaminophen", "hyoscine"]
}
```

**Response:** `200`
```json
{
  "results": [
    {
      "drug_a": "acetaminophen",
      "drug_b": "hyoscine",
      "severity": "none",
      "description": "No se encontraron interacciones conocidas.",
      "recommendation": "Si tiene dudas, consulte con su médico.",
      "source": "none"
    }
  ]
}
```

Severidades posibles: `none`, `mild`, `moderate`, `severe`.
Sources posibles: `dataset`, `ai_fallback`, `none`.

---

## Riesgos y Mitigaciones

| Riesgo                                            | Mitigación                                                              |
| ------------------------------------------------- | ----------------------------------------------------------------------- |
| API de interacciones NLM discontinuada (ene 2024) | CSV Kaggle DDI como fuente primaria + Gemini fallback                   |
| No existe dataset argentino de interacciones       | Kaggle DDI cubre interacciones genéricas internacionales                |
| Rate limits / quota de Gemini                      | Fallback a echo mode cuando AI no disponible                            |
| Datasets argentinos incompletos                    | Diccionario manual 50+ marcas + fuzzy matching + AI fallback            |
| Loop de tool-calls                                 | Cap en 5 rondas por mensaje                                            |
| Typos del usuario                                  | Levenshtein (dist ≤ 2) + prefix match + AI normalization               |

---

## Verificación

1. `docker compose up -d` → PostgreSQL levanta
2. `make run` → servidor en :8080
3. `POST /api/v1/guests` → guest creado con UUID
4. `POST /api/v1/chat/message` con "hola" → AI saluda y pregunta modo
5. `POST /api/v1/chat/message` con "por voz" → tool_call `select_mode` con `voice`
6. `POST /api/v1/chat/message` con "tomo tafirol y buscapina" → tool_call `normalize_medications`
7. `POST /api/v1/chat/message` con "sí, están bien" → tool_call `check_interactions` (señal)
8. `POST /api/v1/interactions/check` con `["acetaminophen", "hyoscine"]` → resultados

package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/guarda/backend/internal/repository"
	"github.com/guarda/backend/internal/service"
	"github.com/guarda/backend/internal/toolcall"
)

type ChatHandler struct {
	convRepo *repository.ConversationRepo
	msgRepo  *repository.MessageRepo
	ai       *service.AIService
	executor *toolcall.Executor
}

func NewChatHandler(
	convRepo *repository.ConversationRepo,
	msgRepo *repository.MessageRepo,
	ai *service.AIService,
	executor *toolcall.Executor,
) *ChatHandler {
	return &ChatHandler{
		convRepo: convRepo,
		msgRepo:  msgRepo,
		ai:       ai,
		executor: executor,
	}
}

type chatRequest struct {
	ConversationID *string `json:"conversation_id"`
	Message        string  `json:"message" binding:"required"`
}

type chatResponse struct {
	ConversationID uuid.UUID        `json:"conversation_id"`
	Message        string           `json:"message"`
	ToolCalls      []toolcall.Result `json:"tool_calls"`
}

func (h *ChatHandler) SendMessage(c *gin.Context) {
	guestID := c.MustGet("guest_id").(uuid.UUID)

	var req chatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "message is required"})
		return
	}

	ctx := c.Request.Context()

	// Get or create conversation
	var conv *model.Conversation
	if req.ConversationID != nil && *req.ConversationID != "" {
		convID, err := uuid.Parse(*req.ConversationID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation_id"})
			return
		}
		conv, err = h.convRepo.GetByID(ctx, convID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "conversation not found"})
			return
		}
	} else {
		conv = &model.Conversation{
			GuestID:  guestID,
			FlowType: "general",
			Status:   "active",
		}
		if err := h.convRepo.Create(ctx, conv); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create conversation"})
			return
		}
	}

	// Save user message
	content := req.Message
	userMsg := &model.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        &content,
	}
	if err := h.msgRepo.Create(ctx, userMsg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save message"})
		return
	}

	// Load conversation history
	messages, err := h.msgRepo.GetByConversation(ctx, conv.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load history"})
		return
	}

	// If AI service is nil, echo mode
	if h.ai == nil {
		c.JSON(http.StatusOK, chatResponse{
			ConversationID: conv.ID,
			Message:        "Echo: " + req.Message,
			ToolCalls:      []toolcall.Result{},
		})
		return
	}

	// Send to AI with tool-call loop (max 5 rounds)
	var allToolResults []toolcall.Result
	responseText := ""
	const maxRounds = 5

	for round := 0; round < maxRounds; round++ {
		aiResp, err := h.ai.Chat(ctx, messages)
		if err != nil {
			log.Printf("AI error, falling back to echo: %v", err)
			c.JSON(http.StatusOK, chatResponse{
				ConversationID: conv.ID,
				Message:        "[AI no disponible] Echo: " + req.Message,
				ToolCalls:      []toolcall.Result{},
			})
			return
		}

		// Capture text from AI (even when there are function calls)
		if aiResp.Text != "" {
			responseText = aiResp.Text
		}

		if len(aiResp.FunctionCalls) == 0 {
			break
		}

		// Process function calls
		var frontendCalls []toolcall.Result
		var backendCalls []service.FunctionCall

		for _, fc := range aiResp.FunctionCalls {
			switch fc.Name {
			case "normalize_medications", "save_guest_profile":
				// Execute in backend
				backendCalls = append(backendCalls, fc)
			case "check_interactions":
				// Signal for frontend — no data, frontend has the meds
				frontendCalls = append(frontendCalls, toolcall.Result{
					Name: fc.Name,
					Data: nil,
				})
			default:
				frontendCalls = append(frontendCalls, toolcall.Result{
					Name: fc.Name,
					Data: fc.Args,
				})
			}
		}

		// Execute backend calls
		if len(backendCalls) > 0 {
			results, err := h.executor.Execute(ctx, conv.ID, guestID, backendCalls)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "tool execution error"})
				return
			}
			allToolResults = append(allToolResults, results...)

			// Save results to history for Gemini context
			for _, fc := range backendCalls {
				tcJSON, _ := json.Marshal(fc)
				raw := json.RawMessage(tcJSON)
				assistantMsg := &model.Message{
					ConversationID: conv.ID,
					Role:           "assistant",
					ToolCalls:      &raw,
				}
				h.msgRepo.Create(ctx, assistantMsg)
				messages = append(messages, *assistantMsg)
			}
			for _, result := range results {
				resultJSON, _ := json.Marshal(result.Data)
				resultStr := string(resultJSON)
				funcMsg := &model.Message{
					ConversationID: conv.ID,
					Role:           "function",
					Content:        &resultStr,
					ToolCallID:     &result.Name,
				}
				h.msgRepo.Create(ctx, funcMsg)
				messages = append(messages, *funcMsg)
			}
		}

		// Add frontend-only calls to results
		allToolResults = append(allToolResults, frontendCalls...)

		// Tool calls processed — stop the loop. Results go to frontend.
		break
	}

	// Save assistant text response if any
	if responseText != "" {
		assistantMsg := &model.Message{
			ConversationID: conv.ID,
			Role:           "assistant",
			Content:        &responseText,
		}
		h.msgRepo.Create(ctx, assistantMsg)
	}

	// If AI returned tool_calls but no text, add a default message
	if responseText == "" && len(allToolResults) > 0 {
		responseText = defaultMessageForToolCall(allToolResults[0].Name)
	}

	// Clean up trailing/leading whitespace and newlines
	responseText = strings.TrimSpace(responseText)

	resp := chatResponse{
		ConversationID: conv.ID,
		Message:        responseText,
		ToolCalls:      allToolResults,
	}
	if resp.ToolCalls == nil {
		resp.ToolCalls = []toolcall.Result{}
	}
	c.JSON(http.StatusOK, resp)
}

func defaultMessageForToolCall(name string) string {
	switch name {
	case "save_guest_profile":
		return "¡Dale! Contame, ¿qué medicamentos estás tomando?"
	case "normalize_medications":
		return "Perfecto, encontré estos medicamentos. ¿Son correctos?"
	case "check_interactions":
		return "¡Dale! Esperame un segundito mientras analizo las interacciones entre tus medicamentos..."
	default:
		return ""
	}
}

package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/guarda/backend/internal/model"
	"github.com/guarda/backend/internal/repository"
)

type GuestHandler struct {
	repo *repository.GuestRepo
}

func NewGuestHandler(repo *repository.GuestRepo) *GuestHandler {
	return &GuestHandler{repo: repo}
}

type createGuestRequest struct {
	PreferredMode string `json:"preferred_mode"`
}

func (h *GuestHandler) Create(c *gin.Context) {
	var req createGuestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req.PreferredMode = "text"
	}
	if req.PreferredMode == "" {
		req.PreferredMode = "text"
	}

	guest := &model.Guest{
		ID:            uuid.New(),
		PreferredMode: req.PreferredMode,
	}

	if err := h.repo.Create(c.Request.Context(), guest); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create guest"})
		return
	}

	guest, _ = h.repo.GetByID(c.Request.Context(), guest.ID)
	c.JSON(http.StatusCreated, guest)
}

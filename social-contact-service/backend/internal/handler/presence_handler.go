package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"social-contact-service/backend/internal/repo"
)

type PresenceHandler struct {
	repo repo.InteractionRepo
}

func NewPresenceHandler(r repo.InteractionRepo) *PresenceHandler {
	return &PresenceHandler{repo: r}
}

type socialReq struct {
	DocID string `json:"doc_id" binding:"required"`
}

// 工厂函数
// 输入：PresenceCache接口定义的函数，包括增减和查询
// 输出：gin.HandlerFunc，用于处理请求
// POST 请求的参数是通过请求体传递的，所以需要使用 ShouldBindJSON 方法获取
func (h *PresenceHandler) makePostDocHandler(
	fn func(context.Context, string, uint64) (uint64, error),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 兼容：POST 既可以 JSON body 传 docId，也可以 query/header 传（一些前端会统一用 header 传 docid）
		var req struct {
			DocID string `json:"docId"`
		}
		if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		docID := req.DocID
		if docID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing doc_id"})
			return
		}
		v, ok := c.Get("userId")
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		userID, ok := v.(uint64)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		val, err := fn(c.Request.Context(), docID, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"value": val})
	}
}

// 注意 GET 请求的参数是通过 URL 传递的，所以需要使用 Query 方法获取
func (h *PresenceHandler) makeGetDocHandler(
	fn func(context.Context, string) (uint64, error),
) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 兼容多种前端传参方式：?doc_id= / ?docId= / Header: docid
		docID := c.Query("doc_id")
		if docID == "" {
			docID = c.Query("docId")
		}
		if docID == "" {
			docID = c.GetHeader("docid")
		}
		if docID == "" {
			docID = c.GetHeader("docId")
		}
		if docID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "missing doc_id"})
			return
		}

		val, err := fn(c.Request.Context(), docID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"value": val})
	}
}

// 导出
func (h *PresenceHandler) GetLike() gin.HandlerFunc { return h.makeGetDocHandler(h.repo.GetLike) }
func (h *PresenceHandler) GetQuestionMark() gin.HandlerFunc {
	return h.makeGetDocHandler(h.repo.GetQuestionMark)
}
func (h *PresenceHandler) GetShare() gin.HandlerFunc { return h.makeGetDocHandler(h.repo.GetShare) }

func (h *PresenceHandler) IncrLike() gin.HandlerFunc { return h.makePostDocHandler(h.repo.IncrLike) }
func (h *PresenceHandler) IncrQuestionMark() gin.HandlerFunc {
	return h.makePostDocHandler(h.repo.IncrQuestionMark)
}
func (h *PresenceHandler) IncrShare() gin.HandlerFunc { return h.makePostDocHandler(h.repo.IncrShare) }

func (h *PresenceHandler) DecrLike() gin.HandlerFunc { return h.makePostDocHandler(h.repo.DecrLike) }
func (h *PresenceHandler) DecrQuestionMark() gin.HandlerFunc {
	return h.makePostDocHandler(h.repo.DecrQuestionMark)
}
func (h *PresenceHandler) DecrShare() gin.HandlerFunc { return h.makePostDocHandler(h.repo.DecrShare) }

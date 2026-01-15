package handlers

import (
	"time"

	"github.com/gin-gonic/gin"
)

type Document struct {
	ID       uint64 `json:"id"`
	Title    string `json:"title"`
	OwnerID  uint64 `json:"ownerId"`
	Archived bool   `json:"archived"`
}

var documents = map[uint64]Document{
	1: {ID: 1, Title: "Document 1", OwnerID: 1, Archived: false},
}

func CreateDocument(c *gin.Context) {
	//从gin.Context获取用户信息；gin.Context对每个用户天然隔离
	userId, exists := c.Get("userId")
	if !exists {
		c.JSON(500, gin.H{"error": "User context missing"})
		return
	}

	ownerID, ok := userId.(uint64)
	if !ok {
		c.JSON(500, gin.H{"error": "Invalid user ID format"})
		return
	}

	// 使用 documentid 创建文档
	// 暂时简化
	documentid := uint64(len(documents) + 1)
	documents[documentid] = Document{ID: documentid, OwnerID: ownerID, Archived: false}
	c.JSON(200, gin.H{"docId": documentid, "ownerId": ownerID, "title": "New Document", "createdAt": time.Now().Format(time.RFC3339)})
}

func GetDocument(c *gin.Context) {
	// c.Query() 检索？后
	// c.Param() 检索全部
	userId := c.Query("userId")
	if userId == "" {
		c.JSON(500, gin.H{"error": "User context missing"})
		return
	}

	documentID := c.Param("documentID")
	if documentID == "" {
		c.JSON(500, gin.H{"error": "Document ID missing"})
		return
	}

	c.JSON(200, gin.H{"id": documentID, "userId": userId})
}

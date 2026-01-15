package ws

import (
	// "time"
	"context"
	"fmt"
	"log"
	"slices"
	"strconv"
	"time"

	"collabServer/backend/internal/collab"

	"github.com/gorilla/websocket"
)

type Conn struct {
	ws        *websocket.Conn
	hub       *Hub
	docID     string
	userID    uint64
	username  string
	clientID  string
	clientSeq uint64
	// chan是 Go 的“通道”（channel），是 goroutine 之间通信的队列。send chan ServerMessage 表示一个只能存放 ServerMessage 的队列。
	send chan OutboundMessage
	//协作引擎服务
	svc collab.Service
	// 信号量控制
	sem *collab.SemaphoreControl
}

// 出站消息接口
type OutboundMessage interface {
	MessageType() string
}

// 隐式实现（继承） OutboundMessage 接口
func (m ServerMessage) MessageType() string      { return m.Type }
func (m OpSubmitMessage) MessageType() string    { return m.Type }
func (m OpAppliedMessage) MessageType() string   { return m.Type }
func (m OpBroadcastMessage) MessageType() string { return m.Type }

func NewConn(ws *websocket.Conn, hub *Hub, docID string, userID uint64, username string, svc collab.Service, sem *collab.SemaphoreControl) *Conn {
	return &Conn{ws: ws, hub: hub, docID: docID, userID: userID, username: username, send: make(chan OutboundMessage, 32), svc: svc, sem: sem}
}

func SetDocID(c *Conn, docID string) {
	c.docID = docID
}

func (c *Conn) SendMessage_Enqueue(msg OutboundMessage) {
	// select 语句是 Go 的“多路复用”机制，用于同时监听多个通道操作，并选择其中一个执行。
	// 同时评估所有 case 的通道操作
	// 如果多个 case 都就绪，随机选择一个执行
	select {
	case c.send <- msg:
	default:
		// 如果队列满了，则丢弃消息
	}
}

func (c *Conn) handleOpSubmit(ctx context.Context, msg OpSubmitMessage, authorID uint64) {
	OpSubmitCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	if err := c.sem.Acquire(OpSubmitCtx); err != nil {
		c.SendMessage_Enqueue(ServerMessage{Type: "error", Content: err.Error()})
		return
	}
	defer c.sem.Release()

	_, err := c.svc.Submit(OpSubmitCtx, msg.DocID, authorID,
		msg.BaseRevision, msg.ClientId, msg.ClientSeq, msg.Ops)
	if err != nil {
		c.SendMessage_Enqueue(ServerMessage{Type: "error", Content: err.Error()})
		return
	}
	curr_revision, _ := c.svc.CurrentRevision(ctx, msg.DocID)
	c.SendMessage_Enqueue(OpAppliedMessage{Type: "op_applied", DocID: msg.DocID, BaseRevision: msg.BaseRevision, CurrentRevision: curr_revision, ClientId: msg.ClientId, ClientSeq: msg.ClientSeq})
	c.hub.BroadcastAppliedOp(msg.DocID, c, c.userID, msg.Ops)
}

func (c *Conn) readLoop(ctx context.Context) {
	defer close(c.send)
	for {
		var clientMessage ClientMessage
		if err := c.ws.ReadJSON(&clientMessage); err != nil {
			log.Printf("read json error (user=%d, doc=%s): %v", c.userID, c.docID, err)
			return
		}
		switch clientMessage.Type {
		case "heartbeat":
			// ServerMessage <- ServerMessage{Type: "feedback", Content: "Heartbeat received"}
			// c.ws.WriteJSON(ServerMessage)
			err := c.hub.presence.AddMember(ctx, c.docID, c.userID, c.username, 600*time.Second)
			if err != nil {
				log.Printf("add member error: %v", err)
			}

			members, err := c.hub.presence.GetAliveMembersWithNames(ctx, c.docID)
			if err != nil {
				log.Printf("get members error: %v", err)
			}
			for _, member := range members {
				c.send <- ServerMessage{Type: "presence", Content: fmt.Sprintf("User %d(%s) is online", member.UserID, member.Username)}
			}

			c.send <- ServerMessage{Type: "feedback", Content: "Heartbeat received"}

		case "createDocument":
			docTitle := clientMessage.DocTitle
			log.Printf("username: %s", c.username)
			log.Printf("docTitle: %s", docTitle)
			if err := c.svc.CreateDocument(ctx, c.userID, docTitle); err != nil {
				log.Printf("create document error: %v", err)
				c.send <- ServerMessage{Type: "error", Content: "CREATE_DOC_FAILED"}
				return
			}
			docID, err := c.svc.GetDocumentID(ctx, docTitle)
			if err != nil {
				log.Printf("get document id error: %v", err)
				c.send <- ServerMessage{Type: "error", Content: "GET_DOCID_FAILED"}
				return
			}
			c.hub.presence.AddMember(ctx, docID, c.userID, c.username, 600*time.Second)
			c.send <- ServerMessage{Type: "createDocument", DocID: docID, Content: "Document " + docID + " created by user " + strconv.FormatUint(c.userID, 10)}

		case "joinDocument":
			// 允许客户端在 joinDocument 中指定 docId，用于动态切换房间
			if clientMessage.DocTitle != "" {
				docID, err := c.svc.GetDocumentID(ctx, clientMessage.DocTitle)
				if err != nil {
					log.Printf("get document id error: %v", err)
					c.send <- ServerMessage{Type: "error", Content: "GET_DOCID_FAILED"}
					continue
				}
				if c.docID != "" && c.docID != docID {
					// 先离开旧房间
					c.hub.Leave(c.docID, c)
					c.docID = docID
					SetDocID(c, c.docID)
				} else {
					c.docID = docID
					SetDocID(c, c.docID)
				}
			}

			documents, err := c.hub.presence.GetDocuments(ctx)
			if err != nil {
				log.Printf("get documents error: %v", err)
			}
			if !slices.Contains(documents, c.docID) {
				c.send <- ServerMessage{Type: "joinDocument", DocID: c.docID, Content: "Document " + c.docID + " not found"}
				continue
			}
			c.hub.Join(c.docID, c)
			c.hub.presence.AddMember(ctx, c.docID, c.userID, c.username, 600*time.Second)
			c.send <- ServerMessage{Type: "joinDocument", DocID: c.docID, Content: "Document " + c.docID + " joined by user " + strconv.FormatUint(c.userID, 10)}

		case "show_alive_members":
			// []cache.PresenceMember
			// 只要“所在包不同”，就是两个不同的类型。
			members, err := c.hub.presence.GetAliveMembersWithNames(ctx, c.docID)
			if err != nil {
				log.Printf("get alive members with names error: %v", err)
			}
			member_names := make([]PresenceMember, len(members))
			for i, m := range members {
				member_names[i] = PresenceMember{UserID: m.UserID, Username: m.Username}
			}
			msg := ServerMessage{Type: "show_alive_members", Members: member_names, Content: fmt.Sprintf("Alive members: %v", member_names)}
			c.send <- msg

		case "op_submit":
			msg := OpSubmitMessage{
				Type:         clientMessage.Type,
				DocID:        clientMessage.DocID,
				BaseRevision: clientMessage.BaseRevision,
				ClientId:     clientMessage.ClientId,
				ClientSeq:    clientMessage.ClientSeq,
				Ops:          clientMessage.Ops,
			}
			c.handleOpSubmit(ctx, msg, c.userID)

		case "saveDocument":
			err := c.svc.SaveSnapshot(ctx, clientMessage.DocID)
			if err != nil {
				log.Printf("save document error: %v", err)
				c.send <- ServerMessage{Type: "saveDocument", Content: "Document " + clientMessage.DocID + " save failed"}
			}
			c.send <- ServerMessage{Type: "saveDocument", Content: "Document " + clientMessage.DocID + " saved"}

		case "loadDocumentContent":
			content, revision, err := c.svc.LoadDocumentContent(ctx, clientMessage.DocID)
			if err != nil {
				log.Printf("load document content error: %v", err)
			} else {
				c.send <- ServerMessage{Type: "loadDocumentContent", Content: content, Revision: revision}
			}

		default:
			// 忽略未知类型，或回一条提示
			c.send <- ServerMessage{Type: "ignored", Content: "Unknown message type"}
		}
	}
}

func (c *Conn) writeLoop() {
	// 持续消费通道中的ServerMessage
	for msg := range c.send {
		_ = c.ws.WriteJSON(msg)
	}
}

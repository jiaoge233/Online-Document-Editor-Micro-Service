package cache

import "fmt"

// 键语义：
// - roomKey(docID):           房间候选成员集合（Set<userId>）
// - memberKey(docID,userID):  成员心跳键（String，占位"1"，带 TTL）
// - namesKey(docID):          房间内 userId→username 映射（Hash）
// - cursorKey(docID,userID):  成员光标/选区 JSON（String，带 TTL）

// 房间集合 room:Set
// 心跳键 member:String TTL
// 名字表 names:Hash
// 光标键 cursor:String TTL

const (
	keyRoomFmt   = "presence:room:%s"       // Set<userId>
	keyMemberFmt = "presence:member:%s:%d"  // String "1" with TTL
	keyNamesFmt  = "presence:room:names:%s" // Hash<userId -> username>
	keyCursorFmt = "presence:cursor:%s:%d"  // String JSON with TTL
)

func roomKey(docID string) string                  { return fmt.Sprintf(keyRoomFmt, docID) }
func memberKey(docID string, userID uint64) string { return fmt.Sprintf(keyMemberFmt, docID, userID) }
func namesKey(docID string) string                 { return fmt.Sprintf(keyNamesFmt, docID) }
func cursorKey(docID string, userID uint64) string { return fmt.Sprintf(keyCursorFmt, docID, userID) }

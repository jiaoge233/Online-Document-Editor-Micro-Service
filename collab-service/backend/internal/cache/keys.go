package cache

import "fmt"

// 键语义：
// - roomKey(docID):           房间在线成员（ZSet<userId, expireAtUnix>，score=expireAt）
// - namesKey(docID):          房间内 userId→username 映射（Hash）
// - docsKey():                文档索引集合（Set<docID>）

// 房间集合 room:Set
// 名字表 names:Hash
// 文档索引 docs:Set

const (
	keyRoomFmt  = "presence:room:{docID:%s}"       // ZSet<userId, expireAtUnix>
	keyNamesFmt = "presence:room:names:{docID:%s}" // Hash<userId -> username>
	keyDocsSet  = "presence:docs"                  // Set<docID>
)

func roomKey(docID string) string  { return fmt.Sprintf(keyRoomFmt, docID) }
func namesKey(docID string) string { return fmt.Sprintf(keyNamesFmt, docID) }
func docsKey() string              { return keyDocsSet }

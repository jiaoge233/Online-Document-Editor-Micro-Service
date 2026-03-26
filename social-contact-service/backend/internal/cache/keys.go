package cache

import "fmt"

// 键名
// Like: 点赞的文档ID string
// QuestionMark: 问题标记的文档ID string
// Share: 分享的文档ID string

// LikedUser: 点赞的用户ID set
// QuestionMarkedUser: 问题标记的用户ID set
// SharedUser: 分享的用户ID set

const (
	// 为何要用{}包住tag ：Redis 会对整个 Key 字符串进行 CRC16 哈希计算（仅{}内部的东西），这样包住让同一个对象分配在同一个机器上面，避免 Lua 脚本等出错
	// 例子： Like:{docID:100} -> 计算 Hash(docID:100) 、LikedUser:{docID:100} -> 计算 Hash(docID:100)
	LikeKey         = "Like:{docID:%s}" // Like:{docId}
	QuestionMarkKey = "QuestionMark:{docID:%s}"
	ShareKey        = "Share:{docID:%s}"

	LikedUserKey          = "LikedUser:{docID:%s}" // LikeUsers:{docId}
	QuestionMarkedUserKey = "QuestionMarkedUser:{docID:%s}"
	SharedUserKey         = "SharedUser:{docID:%s}"

	// DocsSetkey: 文档ID的 set
	DocsSetkey = "presence:docs" // Set<docID>
)

func GetLikeKey(docID string) string         { return fmt.Sprintf(LikeKey, docID) }
func GetQuestionMarkKey(docID string) string { return fmt.Sprintf(QuestionMarkKey, docID) }
func GetShareKey(docID string) string        { return fmt.Sprintf(ShareKey, docID) }

func GetLikedUserKey(docID string) string          { return fmt.Sprintf(LikedUserKey, docID) }
func GetQuestionMarkedUserKey(docID string) string { return fmt.Sprintf(QuestionMarkedUserKey, docID) }
func GetSharedUserKey(docID string) string         { return fmt.Sprintf(SharedUserKey, docID) }

func GetDocsSetKey() string { return DocsSetkey }

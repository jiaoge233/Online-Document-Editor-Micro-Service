package cache

import (
	"fmt"
)

const (
	// 核心理念：同一个用户在一个slot上，不同用户尽可能分散在不同slot上面

	// AuthUserFmt 基于 UserID
	// 使用 {%d} 确保同一用户的相关 Key 分布在同一 Slot
	AuthUserFmt = "auth:user:{%d}"

	// AuthLoginFmt 基于 Username ，用于 Login 阶段查找用户
	// 使用 {%s} 确保同一用户名的相关 Key 分布在同一 Slot
	AuthLoginFmt = "auth:login:name:{%s}"
)

func AuthUserKey(userID uint64) string {
	return fmt.Sprintf(AuthUserFmt, userID)
}

func AuthLoginKey(username string) string {
	return fmt.Sprintf(AuthLoginFmt, username)
}

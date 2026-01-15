package authservice

import (
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	// Go的结构体标签需要用反引号
	UserID   uint64 `json:"sub"`
	Username string `json:"username"`
	Type     string `json:"typ"`
	jwt.RegisteredClaims
}

func getSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-secret"
	}
	// 转换为字节切片（uint8[]）
	return []byte(secret)
}

func SignAccessToken(userID uint64, username string, ttl time.Duration) (string, time.Time, error) {
	// jwt.NewWithClaims接收指针作为参数，需要使用&取地址
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Type:     "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	access_token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(getSecret())
	if err != nil {
		return "", time.Time{}, err
	}
	return access_token, time.Now().Add(ttl), nil
}

func SignRefreshToken(userID uint64, username string, ttl time.Duration) (string, time.Time, error) {
	claims := &Claims{
		UserID:   userID,
		Username: username,
		Type:     "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	refresh_token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(getSecret())
	if err != nil {
		return "", time.Time{}, err
	}
	return refresh_token, time.Now().Add(ttl), nil
}

// 解析任意 token（访问/刷新），返回 Claims
func ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return getSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}
	return nil, jwt.ErrTokenInvalidClaims
}

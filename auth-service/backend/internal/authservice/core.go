package authservice

import (
	"context"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"

	"auth-service/backend/internal/user"
)

var (
	ErrInvalidCredentials      = errors.New("invalid credentials")
	ErrInvalidRefreshToken     = errors.New("invalid refresh token")
	ErrInvalidRefreshTokenType = errors.New("invalid refresh token type")
)

type LoginResult struct {
	UserID       uint64
	Username     string
	AccessToken  string
	RefreshToken string
	ExpiresIn    int32
	TokenType    string
}

type RefreshResult struct {
	UserID      uint64
	Username    string
	AccessToken string
	ExpiresIn   int32
	TokenType   string
}

func LoginWithPassword(ctx context.Context, service *user.Service, username, password string) (*LoginResult, error) {
	u, err := service.GetUserByUsername(ctx, username)
	if err != nil || u == nil {
		return nil, ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	accessToken, _, err := SignAccessToken(u.ID, username, 30*time.Minute)
	if err != nil {
		return nil, err
	}

	refreshToken, _, err := SignRefreshToken(u.ID, username, 7*24*time.Hour)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		UserID:       u.ID,
		Username:     username,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    30 * 60,
		TokenType:    "Bearer",
	}, nil
}

func RegisterUser(ctx context.Context, service *user.Service, username, password string) (uint64, error) {
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return 0, err
	}

	return service.CreateUser(ctx, username, passwordHash)
}

func VerifyToken(tokenString string) (*Claims, error) {
	return ParseToken(tokenString)
}

func RefreshTokens(refreshToken string) (*RefreshResult, error) {
	claims, err := ParseToken(refreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}
	if claims.Type != "refresh" {
		return nil, ErrInvalidRefreshTokenType
	}

	newAccessToken, _, err := SignAccessToken(claims.UserID, claims.Username, 30*time.Minute)
	if err != nil {
		return nil, err
	}

	return &RefreshResult{
		UserID:      claims.UserID,
		Username:    claims.Username,
		AccessToken: newAccessToken,
		ExpiresIn:   30 * 60,
		TokenType:   "Bearer",
	}, nil
}

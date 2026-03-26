package grpcapi

import (
	"context"
	"errors"

	authpb "auth-service/backend/gen/authpb"
	"auth-service/backend/internal/authservice"
	"auth-service/backend/internal/user"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	authpb.UnimplementedAuthServiceServer
	service *user.Service
}

func NewServer(service *user.Service) *Server {
	return &Server{service: service}
}

func (s *Server) Login(ctx context.Context, req *authpb.LoginRequest) (*authpb.LoginResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	result, err := authservice.LoginWithPassword(ctx, s.service, req.GetUsername(), req.GetPassword())
	if err != nil {
		if errors.Is(err, authservice.ErrInvalidCredentials) {
			return nil, status.Error(codes.Unauthenticated, "invalid credentials")
		}
		return nil, status.Error(codes.Internal, "login failed")
	}

	return &authpb.LoginResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresIn:    result.ExpiresIn,
		TokenType:    result.TokenType,
		User: &authpb.LoginUser{
			UserId:   result.UserID,
			Username: result.Username,
		},
	}, nil
}

func (s *Server) Register(ctx context.Context, req *authpb.RegisterRequest) (*authpb.RegisterResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	userID, err := authservice.RegisterUser(ctx, s.service, req.GetUsername(), req.GetPassword())
	if err != nil {
		if errors.Is(err, user.ErrUsernameTaken) {
			return nil, status.Error(codes.AlreadyExists, "username already taken")
		}
		return nil, status.Error(codes.Internal, "register failed")
	}

	return &authpb.RegisterResponse{UserId: userID}, nil
}

func (s *Server) VerifyToken(_ context.Context, req *authpb.VerifyTokenRequest) (*authpb.VerifyTokenResponse, error) {
	if req == nil || req.GetToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "token is required")
	}

	claims, err := authservice.VerifyToken(req.GetToken())
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid token")
	}

	var exp int64
	if claims.ExpiresAt != nil {
		exp = claims.ExpiresAt.Time.Unix()
	}

	return &authpb.VerifyTokenResponse{
		UserId:   claims.UserID,
		Username: claims.Username,
		Typ:      claims.Type,
		Exp:      exp,
	}, nil
}

func (s *Server) RefreshToken(_ context.Context, req *authpb.RefreshTokenRequest) (*authpb.RefreshTokenResponse, error) {
	if req == nil || req.GetRefreshToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh token is required")
	}

	result, err := authservice.RefreshTokens(req.GetRefreshToken())
	if err != nil {
		if errors.Is(err, authservice.ErrInvalidRefreshToken) || errors.Is(err, authservice.ErrInvalidRefreshTokenType) {
			return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
		}
		return nil, status.Error(codes.Internal, "refresh failed")
	}

	return &authpb.RefreshTokenResponse{
		AccessToken: result.AccessToken,
		ExpiresIn:   result.ExpiresIn,
		TokenType:   result.TokenType,
		User: &authpb.LoginUser{
			UserId:   result.UserID,
			Username: result.Username,
		},
	}, nil
}

package user

import (
	"context"
	"errors"
	"fmt"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailRegistered = errors.New("email is already registered")
	ErrLoginGeneric    = errors.New("invalid email or password")
)

type UserService struct {
	repo Repository
	jwt  *auth.JWTManager
}

func NewUserService(repo Repository, jwt *auth.JWTManager) *UserService {
	return &UserService{
		repo: repo,
		jwt:  jwt,
	}
}

func (s *UserService) Register(ctx context.Context, req RegisterRequest) (*User, error) {
	existingUser, _ := s.repo.GetUserByEmail(ctx, req.Email)
	if existingUser != nil {
		return nil, ErrEmailRegistered
	}

	passwordHash, err := hashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("error in hashing password: %w", err)
	}

	user, err := s.repo.CreateUser(ctx, req.Email, passwordHash)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func (s *UserService) Login(ctx context.Context, req LoginRequest) (string, error) {
	existingUser, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", ErrLoginGeneric
		}
		return "", err
	}

	if err := comparePassword(existingUser.PasswordHash, req.Password); err != nil {
		return "", ErrLoginGeneric
	}

	signedToken, err := s.jwt.GenerateToken(existingUser.ID)
	if err != nil {
		return "", err
	}

	return signedToken, nil
}

func comparePassword(passwordHash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password)); err != nil {
		return err
	}
	return nil
}

func hashPassword(password string) (string, error) {
	bytePass, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(bytePass), nil
}

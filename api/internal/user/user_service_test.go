package user

import (
	"context"
	"testing"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockUserRepository struct {
	mock.Mock
}

func (m *mockUserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)

	var u *User
	if args.Get(0) != nil {
		u = args.Get(0).(*User)
	}
	return u, args.Error(1)
}

func (m *mockUserRepository) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	args := m.Called(ctx, email, passwordHash)
	var u *User
	if args.Get(0) != nil {
		u = args.Get(0).(*User)
	}
	return u, args.Error(1)
}

func newTestUserService(repo *mockUserRepository) *UserService {
	jwtManager := auth.NewJWTManager("test-secret", time.Hour)
	return NewUserService(repo, jwtManager)
}

func TestUserService_Register_Success(t *testing.T) {
	repo := new(mockUserRepository)

	repo.On("GetUserByEmail", mock.Anything, "new@mail.com").
		Return(nil, pgx.ErrNoRows)

	repo.On("CreateUser", mock.Anything, "new@mail.com", mock.AnythingOfType("string")).
		Return(&User{Email: "new@mail.com"}, nil)

	svc := newTestUserService(repo)

	user, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "new@mail.com",
		Password: "password123",
	})

	require.NoError(t, err)
	require.Equal(t, "new@mail.com", user.Email)

	repo.AssertExpectations(t)
}

func TestUserService_Register_EmailAlreadyRegistered(t *testing.T) {
	repo := new(mockUserRepository)

	repo.On("GetUserByEmail", mock.Anything, "exists@mail.com").
		Return(&User{Email: "exists@mail.com"}, nil)

	svc := newTestUserService(repo)

	user, err := svc.Register(context.Background(), RegisterRequest{
		Email:    "exists@mail.com",
		Password: "password123",
	})

	require.Nil(t, user)
	require.ErrorIs(t, err, ErrEmailRegistered)

	repo.AssertNotCalled(t, "CreateUser", mock.Anything, mock.Anything, mock.Anything)
}

func TestUserService_Login_Success(t *testing.T) {
	repo := new(mockUserRepository)

	hash, err := hashPassword("password123")
	require.NoError(t, err)

	repo.On("GetUserByEmail", mock.Anything, "user@mail.com").
		Return(&User{Email: "user@mail.com", PasswordHash: hash}, nil)

	svc := newTestUserService(repo)

	token, err := svc.Login(context.Background(), LoginRequest{
		Email:    "user@mail.com",
		Password: "password123",
	})

	require.NoError(t, err)
	require.NotEmpty(t, token)
}

func TestUserService_Login_UserNotFound(t *testing.T) {
	repo := new(mockUserRepository)

	repo.On("GetUserByEmail", mock.Anything, "ghost@mail.com").
		Return(nil, pgx.ErrNoRows)

	svc := newTestUserService(repo)

	token, err := svc.Login(context.Background(), LoginRequest{
		Email:    "ghost@mail.com",
		Password: "whatever",
	})

	require.Empty(t, token)

	require.ErrorIs(t, err, ErrLoginGeneric)
}

func TestUserService_Login_WrongPassword(t *testing.T) {
	repo := new(mockUserRepository)

	hash, err := hashPassword("correct-password")
	require.NoError(t, err)

	repo.On("GetUserByEmail", mock.Anything, "user@mail.com").
		Return(&User{Email: "user@mail.com", PasswordHash: hash}, nil)

	svc := newTestUserService(repo)

	token, err := svc.Login(context.Background(), LoginRequest{
		Email:    "user@mail.com",
		Password: "wrong-password",
	})

	require.Empty(t, token)
	require.ErrorIs(t, err, ErrLoginGeneric)
}

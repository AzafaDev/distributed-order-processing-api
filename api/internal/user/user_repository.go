package user

import (
	"context"
	"fmt"

	"github.com/AzafaDev/distributed-order-processing-api/internal/user/sqlc"
)

type Repository interface {
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	CreateUser(ctx context.Context, email, passwordHash string) (*User, error)
}

type UserRepository struct {
	queries *sqlc.Queries
}

func NewUserRepository(q *sqlc.Queries) Repository {
	return &UserRepository{
		queries: q,
	}
}

func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	user, err := r.queries.GetUserByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("error in get user by email: %w", err)
	}
	return &User{
		ID:        user.ID.Bytes,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.Time,
		UpdatedAt: user.UpdatedAt.Time,
	}, nil
}
func (r *UserRepository) CreateUser(ctx context.Context, email, passwordHash string) (*User, error) {
	user, err := r.queries.CreateUser(ctx, sqlc.CreateUserParams{
		Email:        email,
		PasswordHash: passwordHash,
	})
	if err != nil {
		return nil, fmt.Errorf("error in create user: %w", err)
	}
	return &User{
		ID:        user.ID.Bytes,
		Email:     user.Email,
		CreatedAt: user.CreatedAt.Time,
		UpdatedAt: user.UpdatedAt.Time,
	}, nil
}

package product

import (
	"context"
	"errors"
)

var (
	ErrNoProduct = errors.New("no products available")
)

type ProductService struct {
	repo Repository
}

func NewProductService(repo Repository) *ProductService {
	return &ProductService{
		repo: repo,
	}
}

const maxLimit = 100

func (s *ProductService) ListProducts(ctx context.Context, page, limit int) ([]Product, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	offset := (page - 1) * limit
	listProducts, err := s.repo.ListProducts(ctx, limit, offset)

	if err != nil {
		return nil, err
	}

	if len(listProducts) == 0 {
		return nil, ErrNoProduct
	}

	return listProducts, nil
}

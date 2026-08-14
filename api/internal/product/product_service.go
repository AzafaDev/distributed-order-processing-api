package product

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrNoProduct           = errors.New("no products available")
	ErrExistingProductName = errors.New("product's name cannot be same with the exists one")
	ErrProductNotFound     = errors.New("product not found")
	ErrInsufficientStock   = errors.New("insufficient stock")
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

func (s *ProductService) GetProductByID(ctx context.Context, id uuid.UUID) (*Product, error) {
	existingProduct, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return existingProduct, nil
}

func (s *ProductService) CreateProduct(ctx context.Context, req CreateProductRequest) (*Product, error) {
	createdProduct, err := s.repo.CreateProduct(ctx, req.Name, req.Description, req.Price, req.Stock)
	if err != nil {
		return nil, err
	}

	return createdProduct, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, id uuid.UUID, req UpdateProductRequest) (*Product, error) {
	updatedProduct, err := s.repo.UpdateProduct(ctx, id, req.Name, req.Description, req.Price, req.Stock)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}
	return updatedProduct, nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, id uuid.UUID) error {
	effectedRows, err := s.repo.DeleteProductByID(ctx, id)
	if err != nil {
		return err
	}

	if effectedRows == 0 {
		return ErrProductNotFound
	}

	return nil
}

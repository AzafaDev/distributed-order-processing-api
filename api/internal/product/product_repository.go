package product

import (
	"context"

	"github.com/AzafaDev/distributed-order-processing-api/internal/product/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository interface {
	GetProductByID(ctx context.Context, id uuid.UUID) (*Product, error)
	ListProducts(ctx context.Context, limit, offset int) ([]Product, error)
}

type ProductRepository struct {
	queries *sqlc.Queries
}

func NewProductRepository(q *sqlc.Queries) Repository {
	return &ProductRepository{
		queries: q,
	}
}

func (r *ProductRepository) GetProductByID(ctx context.Context, id uuid.UUID) (*Product, error) {

	product, err := r.queries.GetProductByID(ctx, pgtype.UUID{
		Bytes: id,
		Valid: true,
	})
	if err != nil {
		return nil, err
	}

	numeric, err := product.Price.Int64Value()
	if err != nil {
		return nil, err
	}

	return &Product{
		ID:          product.ID.Bytes,
		Name:        product.Name,
		Description: product.Description,
		Price:       int(numeric.Int64),
		Stock:       int(product.Stock),
		CreatedAt:   product.CreatedAt.Time,
		UpdatedAt:   product.UpdatedAt.Time,
	}, nil
}
func (r *ProductRepository) ListProducts(ctx context.Context, limit, offset int) ([]Product, error) {
	sqlcProducts, err := r.queries.ListProducts(ctx, sqlc.ListProductsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return []Product{}, err
	}
	var products []Product
	for _, eachProduct := range sqlcProducts {
		numeric, err := eachProduct.Price.Int64Value()
		if err != nil {
			return []Product{}, err
		}

		product := Product{
			ID:          eachProduct.ID.Bytes,
			Name:        eachProduct.Name,
			Description: eachProduct.Description,
			Price:       int(numeric.Int64),
			Stock:       int(eachProduct.Stock),
			CreatedAt:   eachProduct.CreatedAt.Time,
			UpdatedAt:   eachProduct.UpdatedAt.Time,
		}

		products = append(products, product)
	}
	return products, nil
}

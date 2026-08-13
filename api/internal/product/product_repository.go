package product

import (
	"context"
	"errors"
	"math/big"

	"github.com/AzafaDev/distributed-order-processing-api/internal/product/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type Repository interface {
	GetProductByID(ctx context.Context, id uuid.UUID) (*Product, error)
	ListProducts(ctx context.Context, limit, offset int) ([]Product, error)
	CreateProduct(ctx context.Context, name, desc string, price, stock int) (*Product, error)
	UpdateProduct(ctx context.Context, id uuid.UUID, name, desc *string, price, stock *int) (*Product, error)
	DeleteProductByID(ctx context.Context, id uuid.UUID) (int, error)
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

func (r *ProductRepository) CreateProduct(ctx context.Context, name, desc string, price, stock int) (*Product, error) {
	createdProduct, err := r.queries.CreateProduct(ctx, sqlc.CreateProductParams{
		Name:        name,
		Description: desc,
		Price: pgtype.Numeric{
			Int:   big.NewInt(int64(price)),
			Valid: true,
		},
		Stock: int32(stock),
	})

	var pgErr *pgconn.PgError

	if err != nil {
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, ErrExistingProductName
		}
		return nil, err
	}

	convertedPrice, err := createdProduct.Price.Int64Value()
	if err != nil {
		return nil, err
	}

	return &Product{
		ID:          createdProduct.ID.Bytes,
		Name:        createdProduct.Name,
		Description: createdProduct.Description,
		Price:       int(convertedPrice.Int64),
		Stock:       int(createdProduct.Stock),
		CreatedAt:   createdProduct.CreatedAt.Time,
		UpdatedAt:   createdProduct.UpdatedAt.Time,
	}, nil
}

func (r *ProductRepository) UpdateProduct(ctx context.Context, id uuid.UUID, name, desc *string, price, stock *int) (*Product, error) {
	nameParam := pgtype.Text{}
	if name != nil {
		nameParam = pgtype.Text{String: *name, Valid: true}
	}

	descParam := pgtype.Text{}
	if desc != nil {
		descParam = pgtype.Text{String: *desc, Valid: true}
	}

	priceParam := pgtype.Numeric{}
	if price != nil {
		priceParam = pgtype.Numeric{Int: big.NewInt(int64(*price)), Valid: true}
	}

	stockParam := pgtype.Int4{}
	if stock != nil {
		stockParam = pgtype.Int4{Int32: int32(*stock), Valid: true}
	}

	updatedProduct, err := r.queries.UpdateProduct(ctx, sqlc.UpdateProductParams{
		Name:        nameParam,
		Description: descParam,
		Price:       priceParam,
		Stock:       stockParam,
		ID: pgtype.UUID{
			Bytes: id,
			Valid: true,
		},
	})

	var pgErr *pgconn.PgError
	if err != nil {
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return nil, ErrExistingProductName
		}
		return nil, err
	}

	convertedPrice, err := updatedProduct.Price.Int64Value()
	if err != nil {
		return nil, err
	}

	return &Product{
		ID:          updatedProduct.ID.Bytes,
		Name:        updatedProduct.Name,
		Description: updatedProduct.Description,
		Price:       int(convertedPrice.Int64),
		Stock:       int(updatedProduct.Stock),
		CreatedAt:   updatedProduct.CreatedAt.Time,
		UpdatedAt:   updatedProduct.UpdatedAt.Time,
	}, nil

}

func (r *ProductRepository) DeleteProductByID(ctx context.Context, id uuid.UUID) (int, error) {
	effectedRows, err := r.queries.DeleteProductByID(ctx, pgtype.UUID{
		Bytes: id,
		Valid: true,
	})
	if err != nil {
		return 0, err
	}
	return int(effectedRows), nil
}

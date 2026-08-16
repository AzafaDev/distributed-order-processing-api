package product

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockProductRepository struct {
	mock.Mock
}

func (m *mockProductRepository) GetProductByID(ctx context.Context, id uuid.UUID) (*Product, error) {
	args := m.Called(ctx, id)
	var p *Product
	if args.Get(0) != nil {
		p = args.Get(0).(*Product)
	}
	return p, args.Error(1)
}

func (m *mockProductRepository) ListProducts(ctx context.Context, limit, offset int) ([]Product, error) {
	args := m.Called(ctx, limit, offset)
	var ps []Product
	if args.Get(0) != nil {
		ps = args.Get(0).([]Product)
	}
	return ps, args.Error(1)
}

func (m *mockProductRepository) CreateProduct(ctx context.Context, name, desc string, price, stock int) (*Product, error) {
	args := m.Called(ctx, name, desc, price, stock)
	var p *Product
	if args.Get(0) != nil {
		p = args.Get(0).(*Product)
	}
	return p, args.Error(1)
}

func (m *mockProductRepository) UpdateProduct(ctx context.Context, id uuid.UUID, name, desc *string, price, stock *int) (*Product, error) {
	args := m.Called(ctx, id, name, desc, price, stock)
	var p *Product
	if args.Get(0) != nil {
		p = args.Get(0).(*Product)
	}
	return p, args.Error(1)
}

func (m *mockProductRepository) DeleteProductByID(ctx context.Context, id uuid.UUID) (int, error) {
	args := m.Called(ctx, id)
	return args.Int(0), args.Error(1)
}

func TestProductService_ListProducts_ClampsPageAndLimit(t *testing.T) {
	repo := new(mockProductRepository)

	repo.On("ListProducts", mock.Anything, 100, 0).
		Return([]Product{{Name: "Product A"}}, nil)

	svc := NewProductService(repo)

	products, err := svc.ListProducts(context.Background(), 0, 500)

	require.NoError(t, err)
	require.Len(t, products, 1)
	repo.AssertExpectations(t)
}

func TestProductService_ListProducts_NegativeLimitClampedToOne(t *testing.T) {
	repo := new(mockProductRepository)

	repo.On("ListProducts", mock.Anything, 1, 0).
		Return([]Product{{Name: "Product A"}}, nil)

	svc := NewProductService(repo)

	_, err := svc.ListProducts(context.Background(), 1, -5)

	require.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestProductService_ListProducts_EmptyResultReturnsErrNoProduct(t *testing.T) {
	repo := new(mockProductRepository)

	repo.On("ListProducts", mock.Anything, mock.Anything, mock.Anything).
		Return([]Product{}, nil)

	svc := NewProductService(repo)

	products, err := svc.ListProducts(context.Background(), 1, 10)

	require.Nil(t, products)
	require.ErrorIs(t, err, ErrNoProduct)
}

func TestProductService_GetProductByID_Success(t *testing.T) {
	repo := new(mockProductRepository)
	id := uuid.New()

	repo.On("GetProductByID", mock.Anything, id).
		Return(&Product{ID: id, Name: "Product A"}, nil)

	svc := NewProductService(repo)

	product, err := svc.GetProductByID(context.Background(), id)

	require.NoError(t, err)
	require.Equal(t, id, product.ID)
}

func TestProductService_GetProductByID_NotFound(t *testing.T) {
	repo := new(mockProductRepository)
	id := uuid.New()

	repo.On("GetProductByID", mock.Anything, id).
		Return(nil, pgx.ErrNoRows)

	svc := NewProductService(repo)

	product, err := svc.GetProductByID(context.Background(), id)

	require.Nil(t, product)
	require.ErrorIs(t, err, ErrProductNotFound)
}

func TestProductService_CreateProduct_Success(t *testing.T) {
	repo := new(mockProductRepository)

	repo.On("CreateProduct", mock.Anything, "Product A", "desc", 10000, 5).
		Return(&Product{Name: "Product A", Price: 10000, Stock: 5}, nil)

	svc := NewProductService(repo)

	product, err := svc.CreateProduct(context.Background(), CreateProductRequest{
		Name:        "Product A",
		Description: "desc",
		Price:       10000,
		Stock:       5,
	})

	require.NoError(t, err)
	require.Equal(t, "Product A", product.Name)
}

func TestProductService_CreateProduct_DuplicateName(t *testing.T) {
	repo := new(mockProductRepository)

	repo.On("CreateProduct", mock.Anything, "Product A", "desc", 10000, 5).
		Return(nil, ErrExistingProductName)

	svc := NewProductService(repo)

	product, err := svc.CreateProduct(context.Background(), CreateProductRequest{
		Name:        "Product A",
		Description: "desc",
		Price:       10000,
		Stock:       5,
	})

	require.Nil(t, product)
	require.ErrorIs(t, err, ErrExistingProductName)
}

func TestProductService_UpdateProduct_NotFound(t *testing.T) {
	repo := new(mockProductRepository)
	id := uuid.New()
	newName := "New Name"

	repo.On("UpdateProduct", mock.Anything, id, &newName, (*string)(nil), (*int)(nil), (*int)(nil)).
		Return(nil, pgx.ErrNoRows)

	svc := NewProductService(repo)

	product, err := svc.UpdateProduct(context.Background(), id, UpdateProductRequest{
		Name: &newName,
	})

	require.Nil(t, product)
	require.ErrorIs(t, err, ErrProductNotFound)
}

func TestProductService_DeleteProduct_NotFound(t *testing.T) {
	repo := new(mockProductRepository)
	id := uuid.New()

	repo.On("DeleteProductByID", mock.Anything, id).
		Return(0, nil)

	svc := NewProductService(repo)

	err := svc.DeleteProduct(context.Background(), id)

	require.ErrorIs(t, err, ErrProductNotFound)
}

func TestProductService_DeleteProduct_Success(t *testing.T) {
	repo := new(mockProductRepository)
	id := uuid.New()

	repo.On("DeleteProductByID", mock.Anything, id).
		Return(1, nil)

	svc := NewProductService(repo)

	err := svc.DeleteProduct(context.Background(), id)

	require.NoError(t, err)
}

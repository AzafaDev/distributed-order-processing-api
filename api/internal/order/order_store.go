package order

import (
	"context"
	"errors"

	"github.com/AzafaDev/distributed-order-processing-api/internal/order/sqlc"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrOrderItemsUnvailable = errors.New("no order items is available")
	ErrOrderNotPending      = errors.New("status order is not pending")
)

// Store is the persistence surface of the order domain: individual statements,
// no workflow. Every method is safe to call either on the pool or inside a
// transaction, which is what lets the service decide the transaction boundary
// instead of the storage layer deciding it for them.
type Store interface {
	LockProductForUpdate(ctx context.Context, productID uuid.UUID) (*Product, error)
	DecreaseProductStock(ctx context.Context, productID uuid.UUID, quantity int) (int64, error)
	IncreaseProductStock(ctx context.Context, productID uuid.UUID, quantity int) (int64, error)

	CreateOrder(ctx context.Context, userID uuid.UUID, totalAmount int) (*Order, error)
	CreateOrderItem(ctx context.Context, params CreateOrderItemParams) (*OrderItem, error)
	CreatePayment(ctx context.Context, orderID uuid.UUID, amount int) (*Payment, error)

	LockOrderForUpdate(ctx context.Context, userID, orderID uuid.UUID) (*Order, error)
	GetOrderItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]OrderItem, error)
	UpdateOrderStatus(ctx context.Context, userID, orderID uuid.UUID, status string) (*Order, error)

	GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error)
	GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error)
}

// Repository is a Store that can also run a group of operations atomically.
type Repository interface {
	Store

	// WithinTx runs fn inside a single database transaction, committing when fn
	// returns nil and rolling back otherwise. The Store handed to fn is scoped
	// to that transaction.
	WithinTx(ctx context.Context, fn func(Store) error) error
}

type CreateOrderItemParams struct {
	OrderID   uuid.UUID
	ProductID uuid.UUID
	Quantity  int
	Price     int
}

func pgUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}

type OrderRepository struct {
	q *sqlc.Queries
	p *pgxpool.Pool
}

func NewOrderRepository(q *sqlc.Queries, p *pgxpool.Pool) Repository {
	return &OrderRepository{q: q, p: p}
}

func (r *OrderRepository) WithinTx(ctx context.Context, fn func(Store) error) error {
	tx, err := r.p.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(&OrderRepository{q: r.q.WithTx(tx), p: r.p}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *OrderRepository) LockProductForUpdate(ctx context.Context, productID uuid.UUID) (*Product, error) {
	sqlcProduct, err := r.q.LockProductForUpdate(ctx, pgUUID(productID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, product.ErrProductNotFound
		}
		return nil, err
	}

	return &Product{
		ID:          sqlcProduct.ID.Bytes,
		Name:        sqlcProduct.Name,
		Description: sqlcProduct.Description,
		Price:       int(sqlcProduct.Price),
		Stock:       int(sqlcProduct.Stock),
		CreatedAt:   sqlcProduct.CreatedAt.Time,
		UpdatedAt:   sqlcProduct.UpdatedAt.Time,
	}, nil
}

func (r *OrderRepository) DecreaseProductStock(ctx context.Context, productID uuid.UUID, quantity int) (int64, error) {
	return r.q.DecreaseProductStock(ctx, sqlc.DecreaseProductStockParams{
		Stock: int32(quantity),
		ID:    pgUUID(productID),
	})
}

func (r *OrderRepository) IncreaseProductStock(ctx context.Context, productID uuid.UUID, quantity int) (int64, error) {
	return r.q.IncreaseProductStock(ctx, sqlc.IncreaseProductStockParams{
		Stock: int32(quantity),
		ID:    pgUUID(productID),
	})
}

func (r *OrderRepository) CreateOrder(ctx context.Context, userID uuid.UUID, totalAmount int) (*Order, error) {
	sqlcOrder, err := r.q.CreateOrder(ctx, sqlc.CreateOrderParams{
		UserID:      pgUUID(userID),
		TotalAmount: int64(totalAmount),
	})
	if err != nil {
		return nil, err
	}

	return orderFromSqlc(sqlcOrder.ID, sqlcOrder.UserID, sqlcOrder.Status,
		sqlcOrder.TotalAmount, sqlcOrder.CreatedAt, sqlcOrder.UpdatedAt), nil
}

func (r *OrderRepository) CreateOrderItem(ctx context.Context, params CreateOrderItemParams) (*OrderItem, error) {
	sqlcOrderItem, err := r.q.CreateOrderItem(ctx, sqlc.CreateOrderItemParams{
		OrderID:   pgUUID(params.OrderID),
		ProductID: pgUUID(params.ProductID),
		Quantity:  int32(params.Quantity),
		Price:     int64(params.Price),
		Subtotal:  int64(params.Price) * int64(params.Quantity),
	})
	if err != nil {
		return nil, err
	}

	return &OrderItem{
		ID:        sqlcOrderItem.ID.Bytes,
		OrderID:   sqlcOrderItem.OrderID.Bytes,
		ProductID: sqlcOrderItem.ProductID.Bytes,
		Quantity:  int(sqlcOrderItem.Quantity),
		Price:     int(sqlcOrderItem.Price),
		SubTotal:  int(sqlcOrderItem.Subtotal),
		CreatedAt: sqlcOrderItem.CreatedAt.Time,
		UpdatedAt: sqlcOrderItem.UpdatedAt.Time,
	}, nil
}

func (r *OrderRepository) CreatePayment(ctx context.Context, orderID uuid.UUID, amount int) (*Payment, error) {
	sqlcPayment, err := r.q.CreatePayment(ctx, sqlc.CreatePaymentParams{
		OrderID:       pgUUID(orderID),
		Amount:        int64(amount),
		Provider:      "fake",
		TransactionID: pgtype.Text{Valid: false},
	})
	if err != nil {
		return nil, err
	}

	return &Payment{
		ID:            sqlcPayment.ID.Bytes,
		OrderID:       sqlcPayment.OrderID.Bytes,
		Amount:        int(sqlcPayment.Amount),
		Status:        sqlcPayment.Status,
		TransactionID: &sqlcPayment.TransactionID.String,
		Provider:      sqlcPayment.Provider,
		CreatedAt:     sqlcPayment.CreatedAt.Time,
		UpdatedAt:     sqlcPayment.UpdatedAt.Time,
	}, nil
}

func (r *OrderRepository) LockOrderForUpdate(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	sqlcOrder, err := r.q.LockOrderForUpdate(ctx, sqlc.LockOrderForUpdateParams{
		UserID: pgUUID(userID),
		ID:     pgUUID(orderID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	return orderFromSqlc(sqlcOrder.ID, sqlcOrder.UserID, sqlcOrder.Status,
		sqlcOrder.TotalAmount, sqlcOrder.CreatedAt, sqlcOrder.UpdatedAt), nil
}

func (r *OrderRepository) GetOrderItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]OrderItem, error) {
	sqlcOrderItems, err := r.q.GetOrderItemsByOrderID(ctx, pgUUID(orderID))
	if err != nil {
		return nil, err
	}

	orderItems := make([]OrderItem, 0, len(sqlcOrderItems))
	for _, item := range sqlcOrderItems {
		orderItems = append(orderItems, OrderItem{
			ID:        item.ID.Bytes,
			OrderID:   item.OrderID.Bytes,
			ProductID: item.ProductID.Bytes,
			Quantity:  int(item.Quantity),
			Price:     int(item.Price),
			SubTotal:  int(item.Subtotal),
			CreatedAt: item.CreatedAt.Time,
			UpdatedAt: item.UpdatedAt.Time,
		})
	}

	return orderItems, nil
}

func (r *OrderRepository) UpdateOrderStatus(ctx context.Context, userID, orderID uuid.UUID, status string) (*Order, error) {
	updatedOrder, err := r.q.UpdateOrderStatus(ctx, sqlc.UpdateOrderStatusParams{
		Status: status,
		ID:     pgUUID(orderID),
		UserID: pgUUID(userID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	return orderFromSqlc(updatedOrder.ID, updatedOrder.UserID, updatedOrder.Status,
		updatedOrder.TotalAmount, updatedOrder.CreatedAt, updatedOrder.UpdatedAt), nil
}

func (r *OrderRepository) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	sqlcOrders, err := r.q.GetOrders(ctx, pgUUID(userID))
	if err != nil {
		return nil, err
	}

	// A user with no orders is a valid answer, not a failure: return an empty
	// (non-nil) slice so it serialises as [] rather than null.
	orders := make([]Order, 0, len(sqlcOrders))
	for _, sqlcOrder := range sqlcOrders {
		orders = append(orders, *orderFromSqlc(sqlcOrder.ID, pgUUID(userID), sqlcOrder.Status,
			sqlcOrder.TotalAmount, sqlcOrder.CreatedAt, sqlcOrder.UpdatedAt))
	}

	return orders, nil
}

func (r *OrderRepository) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	sqlcOrder, err := r.q.GetOrderByID(ctx, sqlc.GetOrderByIDParams{
		UserID: pgUUID(userID),
		ID:     pgUUID(orderID),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	return orderFromSqlc(sqlcOrder.ID, sqlcOrder.UserID, sqlcOrder.Status,
		sqlcOrder.TotalAmount, sqlcOrder.CreatedAt, sqlcOrder.UpdatedAt), nil
}

func orderFromSqlc(id, userID pgtype.UUID, status string, totalAmount int64, createdAt, updatedAt pgtype.Timestamptz) *Order {
	return &Order{
		ID:          id.Bytes,
		UserID:      userID.Bytes,
		Status:      status,
		TotalAmount: int(totalAmount),
		CreatedAt:   createdAt.Time,
		UpdatedAt:   updatedAt.Time,
	}
}

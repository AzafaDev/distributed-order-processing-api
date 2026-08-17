package order

import (
	"bytes"
	"context"
	"errors"
	"math"
	"sort"

	"github.com/AzafaDev/distributed-order-processing-api/internal/order/sqlc"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrOrderNotFound        = errors.New("order not found")
	ErrOrderUnvailable      = errors.New("no order is available")
	ErrOrderItemsUnvailable = errors.New("no order items is available")
	ErrOrderNotPending      = errors.New("status order is not pending")
)

type Repository interface {
	PlaceOrder(ctx context.Context, userID uuid.UUID, items []CreateOrderItemRequest) (*Order, []OrderItem, *Payment, error)
	GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error)
	GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error)
	CancelOrder(ctx context.Context, userID, orderID uuid.UUID) (*Order, error)
}

type OrderRepository struct {
	q *sqlc.Queries
	p *pgxpool.Pool
}

func NewOrderRepository(q *sqlc.Queries, p *pgxpool.Pool) Repository {
	return &OrderRepository{
		q: q,
		p: p,
	}
}

func (r *OrderRepository) PlaceOrder(ctx context.Context, userID uuid.UUID, items []CreateOrderItemRequest) (*Order, []OrderItem, *Payment, error) {
	orderItemsResponse := []OrderItem{}
	var orderResponse Order
	var paymentResponse Payment

	tx, err := r.p.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, nil, err
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	totalPrice := 0
	mapProductPrice := make(map[uuid.UUID]int)
	qtyByProduct := make(map[uuid.UUID]int)

	for _, item := range items {
		qtyByProduct[item.ProductID] += item.Quantity
	}

	keysProduct := []uuid.UUID{}
	for key := range qtyByProduct {
		keysProduct = append(keysProduct, key)
	}

	sort.Slice(keysProduct, func(i, j int) bool {
		return bytes.Compare(keysProduct[i][:], keysProduct[j][:]) < 0
	})

	for _, productID := range keysProduct {
		sqlcProduct, err := qtx.LockProductForUpdate(ctx, pgtype.UUID{
			Bytes: productID,
			Valid: true,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, nil, product.ErrProductNotFound
			}
			return nil, nil, nil, err
		}
		if sqlcProduct.Stock < int32(qtyByProduct[productID]) {
			return nil, nil, nil, product.ErrInsufficientStock
		}

		productPrice, err := sqlcProduct.Price.Float64Value()
		if err != nil {
			return nil, nil, nil, err
		}

		roundedPrice := int(math.Round(productPrice.Float64))

		totalPrice += roundedPrice * qtyByProduct[productID]
		mapProductPrice[sqlcProduct.ID.Bytes] = roundedPrice
	}

	sqlcOrder, err := qtx.CreateOrder(ctx, sqlc.CreateOrderParams{
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
		TotalAmount: int64(totalPrice),
	})
	if err != nil {
		return nil, nil, nil, err
	}

	orderResponse = Order{
		ID:          sqlcOrder.ID.Bytes,
		UserID:      userID,
		Status:      sqlcOrder.Status,
		TotalAmount: int(sqlcOrder.TotalAmount),
		CreatedAt:   sqlcOrder.CreatedAt.Time,
		UpdatedAt:   sqlcOrder.UpdatedAt.Time,
	}

	for _, orderItem := range items {
		sqlcOrderItem, err := qtx.CreateOrderItem(ctx, sqlc.CreateOrderItemParams{
			OrderID: sqlcOrder.ID,
			ProductID: pgtype.UUID{
				Bytes: orderItem.ProductID,
				Valid: true,
			},
			Quantity: int32(orderItem.Quantity),
			Price:    int64(mapProductPrice[orderItem.ProductID]),
			Subtotal: int64(mapProductPrice[orderItem.ProductID]) * int64(orderItem.Quantity),
		})
		if err != nil {
			return nil, nil, nil, err
		}

		orderItemsResponse = append(orderItemsResponse, OrderItem{
			ID:        sqlcOrderItem.ID.Bytes,
			OrderID:   sqlcOrder.ID.Bytes,
			ProductID: orderItem.ProductID,
			Quantity:  int(sqlcOrderItem.Quantity),
			Price:     int(sqlcOrderItem.Price),
			SubTotal:  int(sqlcOrderItem.Subtotal),
			CreatedAt: sqlcOrder.CreatedAt.Time,
			UpdatedAt: sqlcOrder.UpdatedAt.Time,
		})

		rowsAffected, err := qtx.DecreaseProductStock(ctx, sqlc.DecreaseProductStockParams{
			Stock: sqlcOrderItem.Quantity,
			ID:    sqlcOrderItem.ProductID,
		})
		if err != nil {
			return nil, nil, nil, err
		}
		if rowsAffected == 0 {
			return nil, nil, nil, product.ErrInsufficientStock
		}
	}

	sqlcPayment, err := qtx.CreatePayment(ctx, sqlc.CreatePaymentParams{
		OrderID:  sqlcOrder.ID,
		Amount:   sqlcOrder.TotalAmount,
		Provider: "fake",
		TransactionID: pgtype.Text{
			Valid: false,
		},
	})

	if err != nil {
		return nil, nil, nil, err
	}

	paymentResponse = Payment{
		ID:            sqlcPayment.ID.Bytes,
		OrderID:       sqlcPayment.OrderID.Bytes,
		Amount:        int(sqlcPayment.Amount),
		Status:        sqlcPayment.Status,
		TransactionID: &sqlcPayment.TransactionID.String,
		Provider:      sqlcPayment.Provider,
		CreatedAt:     sqlcPayment.CreatedAt.Time,
		UpdatedAt:     sqlcPayment.UpdatedAt.Time,
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, nil, err
	}

	return &orderResponse, orderItemsResponse, &paymentResponse, nil
}

func (r *OrderRepository) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	var ordersReponse []Order

	sqlcOrders, err := r.q.GetOrders(ctx, pgtype.UUID{
		Bytes: userID,
		Valid: true,
	})
	if err != nil {
		return nil, err
	}
	if len(sqlcOrders) == 0 {
		return nil, ErrOrderUnvailable
	}

	for _, sqlcOrder := range sqlcOrders {
		orderRes := Order{
			ID:          sqlcOrder.ID.Bytes,
			UserID:      userID,
			Status:      sqlcOrder.Status,
			TotalAmount: int(sqlcOrder.TotalAmount),
			CreatedAt:   sqlcOrder.CreatedAt.Time,
			UpdatedAt:   sqlcOrder.UpdatedAt.Time,
		}
		ordersReponse = append(ordersReponse, orderRes)
	}

	return ordersReponse, nil
}

func (r *OrderRepository) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	sqlcOrder, err := r.q.GetOrderByID(ctx, sqlc.GetOrderByIDParams{
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
		ID: pgtype.UUID{
			Bytes: orderID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	return &Order{
		ID:          sqlcOrder.ID.Bytes,
		UserID:      sqlcOrder.UserID.Bytes,
		Status:      sqlcOrder.Status,
		TotalAmount: int(sqlcOrder.TotalAmount),
		CreatedAt:   sqlcOrder.CreatedAt.Time,
		UpdatedAt:   sqlcOrder.UpdatedAt.Time,
	}, nil
}

func (r *OrderRepository) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	tx, err := r.p.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	qtx := r.q.WithTx(tx)

	sqlcOrder, err := qtx.LockOrderForUpdate(ctx, sqlc.LockOrderForUpdateParams{
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
		ID: pgtype.UUID{
			Bytes: orderID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if sqlcOrder.Status != "pending" {
		return nil, ErrOrderNotPending
	}

	sqlcOrderItems, err := qtx.GetOrderItemsByOrderID(ctx, sqlcOrder.ID)
	if err != nil {
		return nil, err
	}
	if len(sqlcOrderItems) == 0 {
		return nil, ErrOrderItemsUnvailable
	}

	sort.Slice(sqlcOrderItems, func(i, j int) bool {
		return bytes.Compare(sqlcOrderItems[i].ProductID.Bytes[:], sqlcOrderItems[j].ProductID.Bytes[:]) < 0
	})

	for _, sqlcOrderItem := range sqlcOrderItems {
		sqlcProduct, err := qtx.LockProductForUpdate(ctx, sqlcOrderItem.ProductID)
		if err != nil {
			return nil, err
		}

		affectedRows, err := qtx.IncreaseProductStock(ctx, sqlc.IncreaseProductStockParams{
			Stock: sqlcOrderItem.Quantity,
			ID:    sqlcProduct.ID,
		})
		if err != nil {
			return nil, err
		}
		if affectedRows == 0 {
			return nil, product.ErrInsufficientStock
		}
	}

	updatedOrder, err := qtx.UpdateOrderStatus(ctx, sqlc.UpdateOrderStatusParams{
		Status: "cancelled",
		ID:     sqlcOrder.ID,
		UserID: sqlcOrder.UserID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &Order{
		ID:          updatedOrder.ID.Bytes,
		UserID:      userID,
		Status:      updatedOrder.Status,
		TotalAmount: int(updatedOrder.TotalAmount),
		CreatedAt:   updatedOrder.CreatedAt.Time,
		UpdatedAt:   updatedOrder.UpdatedAt.Time,
	}, nil
}

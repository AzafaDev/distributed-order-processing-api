package payment

import (
	"context"
	"errors"

	"github.com/AzafaDev/distributed-order-processing-api/internal/payment/sqlc"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNoPaymentFound  = errors.New("payment not found")
	ErrNoOrderFound    = errors.New("order not found")
	ErrOrderNotPayable = errors.New("order cannot be paid")
)

type Repository interface {
	Pay(ctx context.Context, userID, orderID uuid.UUID) (*Payment, *Order, error)
	GetPaymentByOrderID(ctx context.Context, userID, orderID uuid.UUID) (*Payment, error)
}

type PaymentRepository struct {
	queries *sqlc.Queries
	pool    *pgxpool.Pool
}

func NewPaymentRepository(q *sqlc.Queries, p *pgxpool.Pool) Repository {
	return &PaymentRepository{
		queries: q,
		pool:    p,
	}
}

func (r *PaymentRepository) Pay(ctx context.Context, userID, orderID uuid.UUID) (*Payment, *Order, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)

	qtx := r.queries.WithTx(tx)

	sqlcOrder, err := qtx.LockOrderForPayment(ctx, sqlc.LockOrderForPaymentParams{
		ID: pgtype.UUID{
			Bytes: orderID,
			Valid: true,
		},
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNoOrderFound
		}
		return nil, nil, err
	}

	sqlcPayment, err := qtx.LockPaymentForUpdate(ctx, sqlcOrder.ID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNoPaymentFound
		}
		return nil, nil, err
	}

	switch sqlcOrder.Status {
	case "paid":
		return &Payment{
				ID:            sqlcPayment.ID.Bytes,
				OrderID:       sqlcPayment.OrderID.Bytes,
				Amount:        int(sqlcPayment.Amount),
				Status:        sqlcPayment.Status,
				TransactionID: transactionIDPtr(sqlcPayment.TransactionID),
				Provider:      sqlcPayment.Provider,
				CreatedAt:     sqlcPayment.CreatedAt.Time,
				UpdatedAt:     sqlcPayment.UpdatedAt.Time,
			}, &Order{
				ID:          sqlcOrder.ID.Bytes,
				UserID:      sqlcOrder.UserID.Bytes,
				Status:      sqlcOrder.Status,
				TotalAmount: int(sqlcOrder.TotalAmount),
				CreatedAt:   sqlcOrder.CreatedAt.Time,
				UpdatedAt:   sqlcOrder.UpdatedAt.Time,
			}, nil
	case "cancelled":
		return nil, nil, ErrOrderNotPayable
	}

	if sqlcPayment.Status == "success" {
		return &Payment{
				ID:            sqlcPayment.ID.Bytes,
				OrderID:       sqlcPayment.OrderID.Bytes,
				Amount:        int(sqlcPayment.Amount),
				Status:        sqlcPayment.Status,
				TransactionID: transactionIDPtr(sqlcPayment.TransactionID),
				Provider:      sqlcPayment.Provider,
				CreatedAt:     sqlcPayment.CreatedAt.Time,
				UpdatedAt:     sqlcPayment.UpdatedAt.Time,
			}, &Order{
				ID:          sqlcOrder.ID.Bytes,
				UserID:      sqlcOrder.UserID.Bytes,
				Status:      sqlcOrder.Status,
				TotalAmount: int(sqlcOrder.TotalAmount),
				CreatedAt:   sqlcOrder.CreatedAt.Time,
				UpdatedAt:   sqlcOrder.UpdatedAt.Time,
			}, nil
	}

	success, transactionID := Charge(int(sqlcPayment.Amount))
	sqlcPayment, err = qtx.UpdatePaymentStatus(ctx, sqlc.UpdatePaymentStatusParams{
		Status: func() string {
			if success {
				return "success"
			}
			return "failed"
		}(),
		TransactionID: pgtype.Text{
			String: transactionID,
			Valid:  true,
		},
		OrderID: sqlcOrder.ID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, ErrNoPaymentFound
		}
		return nil, nil, err
	}

	if sqlcPayment.Status == "success" {
		sqlcOrder, err = qtx.MarkOrderAsPaid(ctx, sqlcOrder.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, nil, ErrNoOrderFound
			}
			return nil, nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	return &Payment{
			ID:            sqlcPayment.ID.Bytes,
			OrderID:       sqlcPayment.OrderID.Bytes,
			Amount:        int(sqlcPayment.Amount),
			Status:        sqlcPayment.Status,
			TransactionID: transactionIDPtr(sqlcPayment.TransactionID),
			Provider:      sqlcPayment.Provider,
			CreatedAt:     sqlcPayment.CreatedAt.Time,
			UpdatedAt:     sqlcPayment.UpdatedAt.Time,
		}, &Order{
			ID:          sqlcOrder.ID.Bytes,
			UserID:      sqlcOrder.UserID.Bytes,
			Status:      sqlcOrder.Status,
			TotalAmount: int(sqlcOrder.TotalAmount),
			CreatedAt:   sqlcOrder.CreatedAt.Time,
			UpdatedAt:   sqlcOrder.UpdatedAt.Time,
		}, nil

}

func (r *PaymentRepository) GetPaymentByOrderID(ctx context.Context, userID, orderID uuid.UUID) (*Payment, error) {
	sqlcPayment, err := r.queries.GetPaymentByOrderID(ctx, sqlc.GetPaymentByOrderIDParams{
		ID: pgtype.UUID{
			Bytes: orderID,
			Valid: true,
		},
		UserID: pgtype.UUID{
			Bytes: userID,
			Valid: true,
		},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNoPaymentFound
		}
		return nil, err
	}

	return &Payment{
		ID:            sqlcPayment.ID.Bytes,
		OrderID:       sqlcPayment.OrderID.Bytes,
		Amount:        int(sqlcPayment.Amount),
		Status:        sqlcPayment.Status,
		TransactionID: transactionIDPtr(sqlcPayment.TransactionID),
		Provider:      sqlcPayment.Provider,
		CreatedAt:     sqlcPayment.CreatedAt.Time,
		UpdatedAt:     sqlcPayment.UpdatedAt.Time,
	}, nil
}

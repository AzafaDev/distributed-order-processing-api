package payment

import (
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Payment struct {
	ID            uuid.UUID `json:"id"`
	OrderID       uuid.UUID `json:"order_id"`
	Amount        int       `json:"amount"`
	Status        string    `json:"status"`
	TransactionID *string   `json:"transaction_id,omitempty"`
	Provider      string    `json:"provider"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type Order struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Status      string    `json:"status"`
	TotalAmount int       `json:"total_amount"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PayResponse struct {
	Payment Payment `json:"payment"`
	Order   Order   `json:"order"`
}

func transactionIDPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

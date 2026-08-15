package payment

import (
	"context"

	"github.com/google/uuid"
)

type PaymentService struct {
	repo Repository
}

func NewPaymentService(repo Repository) *PaymentService {
	return &PaymentService{
		repo: repo,
	}
}

func (s *PaymentService) Pay(ctx context.Context, userID, orderID uuid.UUID) (*Payment, *Order, error) {
	payment, order, err := s.repo.Pay(ctx, userID, orderID)
	if err != nil {
		return nil, nil, err
	}
	return payment, order, nil
}

func (s *PaymentService) GetPaymentByOrderID(ctx context.Context, userID, orderID uuid.UUID) (*Payment, error) {
	payment, err := s.repo.GetPaymentByOrderID(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	return payment, nil
}

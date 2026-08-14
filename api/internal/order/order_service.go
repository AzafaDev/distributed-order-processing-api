package order

import (
	"context"

	"github.com/google/uuid"
)

type OrderService struct {
	r Repository
}

func NewOrderService(r Repository) *OrderService {
	return &OrderService{
		r: r,
	}
}

func (s *OrderService) CreateOrder(ctx context.Context, userID uuid.UUID, req CreateOrderRequest) (*Order, []OrderItem, *Payment, error) {
	createdOrder, createdOrderItems, createdPayment, err := s.r.PlaceOrder(ctx, userID, req.Items)
	if err != nil {
		return nil, nil, nil, err
	}
	return createdOrder, createdOrderItems, createdPayment, nil
}

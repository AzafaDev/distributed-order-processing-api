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

func (s *OrderService) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	orders, err := s.r.GetOrders(ctx, userID)
	if err != nil {
		return nil, err
	}
	return orders, nil
}

func (s *OrderService) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	order, err := s.r.GetOrderByID(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	return order, nil
}

func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	cancelledOrder, err := s.r.CancelOrder(ctx, userID, orderID)
	if err != nil {
		return nil, err
	}
	return cancelledOrder, nil
}

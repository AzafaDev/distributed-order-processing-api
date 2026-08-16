package order

import (
	"context"
	"testing"

	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockOrderRepository struct {
	mock.Mock
}

func (m *mockOrderRepository) PlaceOrder(ctx context.Context, userID uuid.UUID, items []CreateOrderItemRequest) (*Order, []OrderItem, *Payment, error) {
	args := m.Called(ctx, userID, items)

	var o *Order
	var orderItems []OrderItem
	var p *Payment

	if args.Get(0) != nil {
		o = args.Get(0).(*Order)
	}
	if args.Get(1) != nil {
		orderItems = args.Get(1).([]OrderItem)
	}
	if args.Get(2) != nil {
		p = args.Get(2).(*Payment)
	}

	return o, orderItems, p, args.Error(3)
}

func (m *mockOrderRepository) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	args := m.Called(ctx, userID)
	var os []Order
	if args.Get(0) != nil {
		os = args.Get(0).([]Order)
	}
	return os, args.Error(1)
}

func (m *mockOrderRepository) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	args := m.Called(ctx, userID, orderID)
	var o *Order
	if args.Get(0) != nil {
		o = args.Get(0).(*Order)
	}
	return o, args.Error(1)
}

func (m *mockOrderRepository) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	args := m.Called(ctx, userID, orderID)
	var o *Order
	if args.Get(0) != nil {
		o = args.Get(0).(*Order)
	}
	return o, args.Error(1)
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	repo := new(mockOrderRepository)
	userID := uuid.New()
	productID := uuid.New()

	items := []CreateOrderItemRequest{
		{ProductID: productID, Quantity: 2},
	}

	wantOrder := &Order{ID: uuid.New(), UserID: userID, Status: "pending"}
	wantItems := []OrderItem{{ProductID: productID, Quantity: 2}}
	wantPayment := &Payment{Status: "pending"}

	repo.On("PlaceOrder", mock.Anything, userID, items).
		Return(wantOrder, wantItems, wantPayment, nil)

	svc := NewOrderService(repo)

	order, orderItems, payment, err := svc.CreateOrder(context.Background(), userID, CreateOrderRequest{
		Items: items,
	})

	require.NoError(t, err)
	require.Equal(t, wantOrder, order)
	require.Equal(t, wantItems, orderItems)
	require.Equal(t, wantPayment, payment)
}

func TestOrderService_CreateOrder_InsufficientStock(t *testing.T) {
	repo := new(mockOrderRepository)
	userID := uuid.New()
	items := []CreateOrderItemRequest{{ProductID: uuid.New(), Quantity: 100}}

	repo.On("PlaceOrder", mock.Anything, userID, items).
		Return(nil, nil, nil, product.ErrInsufficientStock)

	svc := NewOrderService(repo)

	order, orderItems, payment, err := svc.CreateOrder(context.Background(), userID, CreateOrderRequest{
		Items: items,
	})

	require.Nil(t, order)
	require.Nil(t, orderItems)
	require.Nil(t, payment)
	require.ErrorIs(t, err, product.ErrInsufficientStock)
}

func TestOrderService_GetOrders_Success(t *testing.T) {
	repo := new(mockOrderRepository)
	userID := uuid.New()
	want := []Order{{ID: uuid.New(), UserID: userID}}

	repo.On("GetOrders", mock.Anything, userID).Return(want, nil)

	svc := NewOrderService(repo)

	orders, err := svc.GetOrders(context.Background(), userID)

	require.NoError(t, err)
	require.Equal(t, want, orders)
}

func TestOrderService_GetOrders_Empty(t *testing.T) {
	repo := new(mockOrderRepository)
	userID := uuid.New()

	repo.On("GetOrders", mock.Anything, userID).Return(nil, ErrOrderUnvailable)

	svc := NewOrderService(repo)

	orders, err := svc.GetOrders(context.Background(), userID)

	require.Nil(t, orders)
	require.ErrorIs(t, err, ErrOrderUnvailable)
}

func TestOrderService_GetOrderByID_NotFound(t *testing.T) {
	repo := new(mockOrderRepository)
	userID, orderID := uuid.New(), uuid.New()

	repo.On("GetOrderByID", mock.Anything, userID, orderID).
		Return(nil, ErrOrderNotFound)

	svc := NewOrderService(repo)

	order, err := svc.GetOrderByID(context.Background(), userID, orderID)

	require.Nil(t, order)
	require.ErrorIs(t, err, ErrOrderNotFound)
}

func TestOrderService_CancelOrder_Success(t *testing.T) {
	repo := new(mockOrderRepository)
	userID, orderID := uuid.New(), uuid.New()
	want := &Order{ID: orderID, UserID: userID, Status: "cancelled"}

	repo.On("CancelOrder", mock.Anything, userID, orderID).Return(want, nil)

	svc := NewOrderService(repo)

	order, err := svc.CancelOrder(context.Background(), userID, orderID)

	require.NoError(t, err)
	require.Equal(t, "cancelled", order.Status)
}

func TestOrderService_CancelOrder_NotPending(t *testing.T) {
	repo := new(mockOrderRepository)
	userID, orderID := uuid.New(), uuid.New()

	repo.On("CancelOrder", mock.Anything, userID, orderID).
		Return(nil, ErrOrderNotPending)

	svc := NewOrderService(repo)

	order, err := svc.CancelOrder(context.Background(), userID, orderID)

	require.Nil(t, order)
	require.ErrorIs(t, err, ErrOrderNotPending)
}

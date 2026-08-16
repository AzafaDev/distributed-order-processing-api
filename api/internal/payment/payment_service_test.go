package payment

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockPaymentRepository struct {
	mock.Mock
}

func (m *mockPaymentRepository) Pay(ctx context.Context, userID, orderID uuid.UUID) (*Payment, *Order, error) {
	args := m.Called(ctx, userID, orderID)

	var p *Payment
	var o *Order
	if args.Get(0) != nil {
		p = args.Get(0).(*Payment)
	}
	if args.Get(1) != nil {
		o = args.Get(1).(*Order)
	}

	return p, o, args.Error(2)
}

func (m *mockPaymentRepository) GetPaymentByOrderID(ctx context.Context, userID, orderID uuid.UUID) (*Payment, error) {
	args := m.Called(ctx, userID, orderID)
	var p *Payment
	if args.Get(0) != nil {
		p = args.Get(0).(*Payment)
	}
	return p, args.Error(1)
}

func TestPaymentService_Pay_Success(t *testing.T) {
	repo := new(mockPaymentRepository)
	userID, orderID := uuid.New(), uuid.New()

	wantPayment := &Payment{OrderID: orderID, Status: "success"}
	wantOrder := &Order{ID: orderID, Status: "paid"}

	repo.On("Pay", mock.Anything, userID, orderID).
		Return(wantPayment, wantOrder, nil)

	svc := NewPaymentService(repo)

	payment, order, err := svc.Pay(context.Background(), userID, orderID)

	require.NoError(t, err)
	require.Equal(t, wantPayment, payment)
	require.Equal(t, wantOrder, order)
}

func TestPaymentService_Pay_OrderNotFound(t *testing.T) {
	repo := new(mockPaymentRepository)
	userID, orderID := uuid.New(), uuid.New()

	repo.On("Pay", mock.Anything, userID, orderID).
		Return(nil, nil, ErrNoOrderFound)

	svc := NewPaymentService(repo)

	payment, order, err := svc.Pay(context.Background(), userID, orderID)

	require.Nil(t, payment)
	require.Nil(t, order)
	require.ErrorIs(t, err, ErrNoOrderFound)
}

func TestPaymentService_Pay_OrderNotPayable(t *testing.T) {
	repo := new(mockPaymentRepository)
	userID, orderID := uuid.New(), uuid.New()

	repo.On("Pay", mock.Anything, userID, orderID).
		Return(nil, nil, ErrOrderNotPayable)

	svc := NewPaymentService(repo)

	payment, order, err := svc.Pay(context.Background(), userID, orderID)

	require.Nil(t, payment)
	require.Nil(t, order)
	require.ErrorIs(t, err, ErrOrderNotPayable)
}

func TestPaymentService_GetPaymentByOrderID_Success(t *testing.T) {
	repo := new(mockPaymentRepository)
	userID, orderID := uuid.New(), uuid.New()
	want := &Payment{OrderID: orderID, Status: "success"}

	repo.On("GetPaymentByOrderID", mock.Anything, userID, orderID).
		Return(want, nil)

	svc := NewPaymentService(repo)

	payment, err := svc.GetPaymentByOrderID(context.Background(), userID, orderID)

	require.NoError(t, err)
	require.Equal(t, want, payment)
}

func TestPaymentService_GetPaymentByOrderID_NotFound(t *testing.T) {
	repo := new(mockPaymentRepository)
	userID, orderID := uuid.New(), uuid.New()

	repo.On("GetPaymentByOrderID", mock.Anything, userID, orderID).
		Return(nil, ErrNoPaymentFound)

	svc := NewPaymentService(repo)

	payment, err := svc.GetPaymentByOrderID(context.Background(), userID, orderID)

	require.Nil(t, payment)
	require.ErrorIs(t, err, ErrNoPaymentFound)
}

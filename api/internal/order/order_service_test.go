package order

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeStore is an in-memory Store. The service owns the order workflow now, so
// these tests exercise that workflow directly: what gets locked, in what order,
// what gets written, and what is left behind when a transaction rolls back.
type fakeStore struct {
	products map[uuid.UUID]*Product
	orders   map[uuid.UUID]*Order
	items    map[uuid.UUID][]OrderItem
	payments map[uuid.UUID]*Payment

	// lockSequence records every LockProductForUpdate call, in order, so a test
	// can assert the deadlock-avoiding lock ordering.
	lockSequence []uuid.UUID

	commits   int
	rollbacks int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		products: map[uuid.UUID]*Product{},
		orders:   map[uuid.UUID]*Order{},
		items:    map[uuid.UUID][]OrderItem{},
		payments: map[uuid.UUID]*Payment{},
	}
}

func (f *fakeStore) addProduct(price, stock int) uuid.UUID {
	id := uuid.New()
	f.products[id] = &Product{ID: id, Name: id.String(), Price: price, Stock: stock}
	return id
}

// WithinTx snapshots the store and restores it when fn fails, which is what
// makes "nothing was written" assertions meaningful.
func (f *fakeStore) WithinTx(ctx context.Context, fn func(Store) error) error {
	snapshot := map[uuid.UUID]Product{}
	for id, p := range f.products {
		snapshot[id] = *p
	}

	if err := fn(f); err != nil {
		f.rollbacks++
		f.orders = map[uuid.UUID]*Order{}
		f.items = map[uuid.UUID][]OrderItem{}
		f.payments = map[uuid.UUID]*Payment{}
		for id := range snapshot {
			restored := snapshot[id]
			f.products[id] = &restored
		}
		return err
	}

	f.commits++
	return nil
}

func (f *fakeStore) LockProductForUpdate(ctx context.Context, productID uuid.UUID) (*Product, error) {
	f.lockSequence = append(f.lockSequence, productID)

	p, ok := f.products[productID]
	if !ok {
		return nil, product.ErrProductNotFound
	}

	locked := *p
	return &locked, nil
}

func (f *fakeStore) DecreaseProductStock(ctx context.Context, productID uuid.UUID, quantity int) (int64, error) {
	p, ok := f.products[productID]
	if !ok || p.Stock < quantity {
		return 0, nil
	}

	p.Stock -= quantity
	return 1, nil
}

func (f *fakeStore) IncreaseProductStock(ctx context.Context, productID uuid.UUID, quantity int) (int64, error) {
	p, ok := f.products[productID]
	if !ok {
		return 0, nil
	}

	p.Stock += quantity
	return 1, nil
}

func (f *fakeStore) CreateOrder(ctx context.Context, userID uuid.UUID, totalAmount int) (*Order, error) {
	o := &Order{
		ID:          uuid.New(),
		UserID:      userID,
		Status:      "pending",
		TotalAmount: totalAmount,
		CreatedAt:   time.Now(),
	}
	f.orders[o.ID] = o

	return o, nil
}

func (f *fakeStore) CreateOrderItem(ctx context.Context, params CreateOrderItemParams) (*OrderItem, error) {
	item := OrderItem{
		ID:        uuid.New(),
		OrderID:   params.OrderID,
		ProductID: params.ProductID,
		Quantity:  params.Quantity,
		Price:     params.Price,
		SubTotal:  params.Price * params.Quantity,
	}
	f.items[params.OrderID] = append(f.items[params.OrderID], item)

	return &item, nil
}

func (f *fakeStore) CreatePayment(ctx context.Context, orderID uuid.UUID, amount int) (*Payment, error) {
	p := &Payment{ID: uuid.New(), OrderID: orderID, Amount: amount, Status: "pending", Provider: "fake"}
	f.payments[orderID] = p

	return p, nil
}

func (f *fakeStore) LockOrderForUpdate(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	o, ok := f.orders[orderID]
	if !ok || o.UserID != userID {
		return nil, ErrOrderNotFound
	}

	locked := *o
	return &locked, nil
}

func (f *fakeStore) GetOrderItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]OrderItem, error) {
	return f.items[orderID], nil
}

func (f *fakeStore) UpdateOrderStatus(ctx context.Context, userID, orderID uuid.UUID, status string) (*Order, error) {
	o, ok := f.orders[orderID]
	if !ok || o.UserID != userID {
		return nil, ErrOrderNotFound
	}

	o.Status = status
	updated := *o

	return &updated, nil
}

func (f *fakeStore) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	orders := []Order{}
	for _, o := range f.orders {
		if o.UserID == userID {
			orders = append(orders, *o)
		}
	}

	return orders, nil
}

func (f *fakeStore) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	o, ok := f.orders[orderID]
	if !ok || o.UserID != userID {
		return nil, ErrOrderNotFound
	}

	found := *o
	return &found, nil
}

func TestOrderService_CreateOrder_Success(t *testing.T) {
	store := newFakeStore()
	productID := store.addProduct(1500, 10)
	userID := uuid.New()

	svc := NewOrderService(store)

	order, items, payment, err := svc.CreateOrder(context.Background(), userID, CreateOrderRequest{
		Items: []CreateOrderItemRequest{{ProductID: productID, Quantity: 3}},
	})

	require.NoError(t, err)
	require.Equal(t, 4500, order.TotalAmount)
	require.Equal(t, "pending", order.Status)

	require.Len(t, items, 1)
	require.Equal(t, 1500, items[0].Price, "price must be snapshotted from the locked product")
	require.Equal(t, 4500, items[0].SubTotal)

	require.Equal(t, 4500, payment.Amount)
	require.Equal(t, 7, store.products[productID].Stock)
	require.Equal(t, 1, store.commits)
}

// The whole order must be one transaction: a refused order may not leave an
// order row, a payment row, or a stock decrement behind.
func TestOrderService_CreateOrder_InsufficientStock(t *testing.T) {
	store := newFakeStore()
	productID := store.addProduct(1000, 2)

	svc := NewOrderService(store)

	_, _, _, err := svc.CreateOrder(context.Background(), uuid.New(), CreateOrderRequest{
		Items: []CreateOrderItemRequest{{ProductID: productID, Quantity: 5}},
	})

	require.ErrorIs(t, err, product.ErrInsufficientStock)
	require.Equal(t, 2, store.products[productID].Stock)
	require.Empty(t, store.orders)
	require.Empty(t, store.payments)
	require.Equal(t, 1, store.rollbacks)
	require.Equal(t, 0, store.commits)
}

func TestOrderService_CreateOrder_ProductNotFound(t *testing.T) {
	store := newFakeStore()
	svc := NewOrderService(store)

	_, _, _, err := svc.CreateOrder(context.Background(), uuid.New(), CreateOrderRequest{
		Items: []CreateOrderItemRequest{{ProductID: uuid.New(), Quantity: 1}},
	})

	require.ErrorIs(t, err, product.ErrProductNotFound)
	require.Empty(t, store.orders)
}

// Two concurrent orders touching the same products in opposite request order
// would deadlock if each locked rows in the order the client sent them, so the
// service must always lock sorted by product UUID.
func TestOrderService_CreateOrder_LocksProductsInDeterministicOrder(t *testing.T) {
	store := newFakeStore()

	expected := []uuid.UUID{store.addProduct(100, 10), store.addProduct(200, 10), store.addProduct(300, 10)}
	sort.Slice(expected, func(i, j int) bool {
		return bytes.Compare(expected[i][:], expected[j][:]) < 0
	})

	// Send them in the exact reverse of the expected lock order.
	items := make([]CreateOrderItemRequest, 0, len(expected))
	for i := len(expected) - 1; i >= 0; i-- {
		items = append(items, CreateOrderItemRequest{ProductID: expected[i], Quantity: 1})
	}

	svc := NewOrderService(store)

	_, _, _, err := svc.CreateOrder(context.Background(), uuid.New(), CreateOrderRequest{Items: items})

	require.NoError(t, err)
	require.Equal(t, expected, store.lockSequence)
}

// Quantities for the same product must be summed before the stock check,
// otherwise two lines of 3 against a stock of 5 would both pass.
func TestOrderService_CreateOrder_AggregatesDuplicateProductLines(t *testing.T) {
	store := newFakeStore()
	productID := store.addProduct(500, 5)

	svc := NewOrderService(store)

	_, _, _, err := svc.CreateOrder(context.Background(), uuid.New(), CreateOrderRequest{
		Items: []CreateOrderItemRequest{
			{ProductID: productID, Quantity: 3},
			{ProductID: productID, Quantity: 3},
		},
	})

	require.ErrorIs(t, err, product.ErrInsufficientStock)
	require.Equal(t, 5, store.products[productID].Stock)
	require.Len(t, store.lockSequence, 1, "a repeated product must be locked once")
}

func TestOrderService_GetOrders_Success(t *testing.T) {
	store := newFakeStore()
	userID := uuid.New()
	productID := store.addProduct(1000, 10)

	svc := NewOrderService(store)

	_, _, _, err := svc.CreateOrder(context.Background(), userID, CreateOrderRequest{
		Items: []CreateOrderItemRequest{{ProductID: productID, Quantity: 1}},
	})
	require.NoError(t, err)

	orders, err := svc.GetOrders(context.Background(), userID)

	require.NoError(t, err)
	require.Len(t, orders, 1)
}

// A user with no orders is a valid answer, not an error condition.
func TestOrderService_GetOrders_Empty(t *testing.T) {
	store := newFakeStore()
	svc := NewOrderService(store)

	orders, err := svc.GetOrders(context.Background(), uuid.New())

	require.NoError(t, err)
	require.NotNil(t, orders)
	require.Empty(t, orders)
}

func TestOrderService_GetOrderByID_NotFound(t *testing.T) {
	store := newFakeStore()
	svc := NewOrderService(store)

	order, err := svc.GetOrderByID(context.Background(), uuid.New(), uuid.New())

	require.Nil(t, order)
	require.ErrorIs(t, err, ErrOrderNotFound)
}

func TestOrderService_CancelOrder_RestoresStock(t *testing.T) {
	store := newFakeStore()
	userID := uuid.New()
	productID := store.addProduct(1000, 10)

	svc := NewOrderService(store)

	order, _, _, err := svc.CreateOrder(context.Background(), userID, CreateOrderRequest{
		Items: []CreateOrderItemRequest{{ProductID: productID, Quantity: 4}},
	})
	require.NoError(t, err)
	require.Equal(t, 6, store.products[productID].Stock)

	cancelled, err := svc.CancelOrder(context.Background(), userID, order.ID)

	require.NoError(t, err)
	require.Equal(t, "cancelled", cancelled.Status)
	require.Equal(t, 10, store.products[productID].Stock)
}

func TestOrderService_CancelOrder_NotPending(t *testing.T) {
	store := newFakeStore()
	userID := uuid.New()
	productID := store.addProduct(1000, 10)

	svc := NewOrderService(store)

	order, _, _, err := svc.CreateOrder(context.Background(), userID, CreateOrderRequest{
		Items: []CreateOrderItemRequest{{ProductID: productID, Quantity: 1}},
	})
	require.NoError(t, err)

	_, err = svc.CancelOrder(context.Background(), userID, order.ID)
	require.NoError(t, err)

	_, err = svc.CancelOrder(context.Background(), userID, order.ID)

	require.ErrorIs(t, err, ErrOrderNotPending)
	require.Equal(t, 10, store.products[productID].Stock, "a second cancel must not restore stock twice")
}

func TestOrderService_CancelOrder_NotFound(t *testing.T) {
	store := newFakeStore()
	svc := NewOrderService(store)

	_, err := svc.CancelOrder(context.Background(), uuid.New(), uuid.New())

	require.True(t, errors.Is(err, ErrOrderNotFound))
}

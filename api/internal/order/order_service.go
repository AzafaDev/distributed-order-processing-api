package order

import (
	"bytes"
	"context"
	"errors"
	"sort"

	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("github.com/AzafaDev/distributed-order-processing-api/internal/order")

// businessErrors are expected outcomes (the caller asked for something the
// domain refuses), not failures. They are recorded as span attributes rather
// than span errors so that alerting on span errors stays meaningful.
var businessErrors = []error{
	product.ErrProductNotFound,
	product.ErrInsufficientStock,
	ErrOrderNotFound,
	ErrOrderNotPending,
	ErrOrderItemsUnvailable,
}

func endSpan(span trace.Span, err error) {
	defer span.End()

	if err == nil {
		return
	}

	for _, businessErr := range businessErrors {
		if errors.Is(err, businessErr) {
			span.SetAttributes(attribute.String("order.outcome", err.Error()))
			return
		}
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type OrderService struct {
	r Repository
}

func NewOrderService(r Repository) *OrderService {
	return &OrderService{
		r: r,
	}
}

// CreateOrder places an order atomically: it locks every product involved,
// verifies stock, snapshots prices, writes the order, its items and a pending
// payment, and decrements stock — all inside one transaction.
func (s *OrderService) CreateOrder(ctx context.Context, userID uuid.UUID, req CreateOrderRequest) (_ *Order, _ []OrderItem, _ *Payment, err error) {
	ctx, span := tracer.Start(ctx, "order.PlaceOrder")
	defer func() { endSpan(span, err) }()

	var (
		createdOrder   *Order
		createdItems   []OrderItem
		createdPayment *Payment
	)

	err = s.r.WithinTx(ctx, func(st Store) error {
		quantities := quantityByProduct(req.Items)

		prices, totalPrice, err := s.lockAndPrice(ctx, st, quantities)
		if err != nil {
			return err
		}

		createdOrder, err = st.CreateOrder(ctx, userID, totalPrice)
		if err != nil {
			return err
		}

		createdItems = make([]OrderItem, 0, len(req.Items))
		for _, item := range req.Items {
			createdItem, err := st.CreateOrderItem(ctx, CreateOrderItemParams{
				OrderID:   createdOrder.ID,
				ProductID: item.ProductID,
				Quantity:  item.Quantity,
				Price:     prices[item.ProductID],
			})
			if err != nil {
				return err
			}
			createdItems = append(createdItems, *createdItem)

			rowsAffected, err := st.DecreaseProductStock(ctx, item.ProductID, item.Quantity)
			if err != nil {
				return err
			}
			// The row is already locked, so this can only fail if the stock
			// check above was wrong — treat it as the same refusal.
			if rowsAffected == 0 {
				return product.ErrInsufficientStock
			}
		}

		createdPayment, err = st.CreatePayment(ctx, createdOrder.ID, createdOrder.TotalAmount)
		return err
	})
	if err != nil {
		return nil, nil, nil, err
	}

	return createdOrder, createdItems, createdPayment, nil
}

// lockAndPrice locks each product row in a deterministic order and returns the
// price snapshot plus the order total.
//
// The lock order matters: two concurrent orders touching the same pair of
// products in opposite orders would deadlock, so every transaction acquires
// them sorted by product UUID.
func (s *OrderService) lockAndPrice(ctx context.Context, st Store, quantities map[uuid.UUID]int) (_ map[uuid.UUID]int, _ int, err error) {
	ctx, span := tracer.Start(ctx, "order.lockProducts")
	span.SetAttributes(attribute.Int("order.product_count", len(quantities)))
	defer func() { endSpan(span, err) }()

	prices := make(map[uuid.UUID]int, len(quantities))
	totalPrice := 0

	for _, productID := range sortedProductIDs(quantities) {
		lockedProduct, err := st.LockProductForUpdate(ctx, productID)
		if err != nil {
			return nil, 0, err
		}
		if lockedProduct.Stock < quantities[productID] {
			return nil, 0, product.ErrInsufficientStock
		}

		prices[productID] = lockedProduct.Price
		totalPrice += lockedProduct.Price * quantities[productID]
	}

	return prices, totalPrice, nil
}

func (s *OrderService) GetOrders(ctx context.Context, userID uuid.UUID) ([]Order, error) {
	return s.r.GetOrders(ctx, userID)
}

func (s *OrderService) GetOrderByID(ctx context.Context, userID, orderID uuid.UUID) (*Order, error) {
	return s.r.GetOrderByID(ctx, userID, orderID)
}

// CancelOrder returns the ordered stock to the catalog and marks the order
// cancelled, atomically. Only a pending order can be cancelled.
func (s *OrderService) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) (_ *Order, err error) {
	ctx, span := tracer.Start(ctx, "order.CancelOrder")
	defer func() { endSpan(span, err) }()

	var cancelledOrder *Order

	err = s.r.WithinTx(ctx, func(st Store) error {
		lockedOrder, err := st.LockOrderForUpdate(ctx, userID, orderID)
		if err != nil {
			return err
		}
		if lockedOrder.Status != "pending" {
			return ErrOrderNotPending
		}

		orderItems, err := st.GetOrderItemsByOrderID(ctx, orderID)
		if err != nil {
			return err
		}
		if len(orderItems) == 0 {
			return ErrOrderItemsUnvailable
		}

		// Same deterministic lock order as placement, for the same reason.
		sort.Slice(orderItems, func(i, j int) bool {
			return bytes.Compare(orderItems[i].ProductID[:], orderItems[j].ProductID[:]) < 0
		})

		for _, item := range orderItems {
			if _, err := st.LockProductForUpdate(ctx, item.ProductID); err != nil {
				return err
			}

			rowsAffected, err := st.IncreaseProductStock(ctx, item.ProductID, item.Quantity)
			if err != nil {
				return err
			}
			if rowsAffected == 0 {
				return product.ErrProductNotFound
			}
		}

		cancelledOrder, err = st.UpdateOrderStatus(ctx, userID, orderID, "cancelled")
		return err
	})
	if err != nil {
		return nil, err
	}

	return cancelledOrder, nil
}

func quantityByProduct(items []CreateOrderItemRequest) map[uuid.UUID]int {
	quantities := make(map[uuid.UUID]int, len(items))
	for _, item := range items {
		quantities[item.ProductID] += item.Quantity
	}
	return quantities
}

func sortedProductIDs(quantities map[uuid.UUID]int) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(quantities))
	for id := range quantities {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool {
		return bytes.Compare(ids[i][:], ids[j][:]) < 0
	})

	return ids
}

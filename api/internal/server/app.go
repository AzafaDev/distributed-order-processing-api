package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/AzafaDev/distributed-order-processing-api/internal/health"
	"github.com/AzafaDev/distributed-order-processing-api/internal/idempotency"
	idempotencySqlc "github.com/AzafaDev/distributed-order-processing-api/internal/idempotency/sqlc"
	"github.com/AzafaDev/distributed-order-processing-api/internal/order"
	orderSqlc "github.com/AzafaDev/distributed-order-processing-api/internal/order/sqlc"
	"github.com/AzafaDev/distributed-order-processing-api/internal/payment"
	paymentSqlc "github.com/AzafaDev/distributed-order-processing-api/internal/payment/sqlc"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/database"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/metrics"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/ratelimit"
	"github.com/AzafaDev/distributed-order-processing-api/internal/product"
	productSqlc "github.com/AzafaDev/distributed-order-processing-api/internal/product/sqlc"
	"github.com/AzafaDev/distributed-order-processing-api/internal/user"
	userSqlc "github.com/AzafaDev/distributed-order-processing-api/internal/user/sqlc"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
)

type App struct {
	Router  http.Handler
	DB      *database.DB
	Redis   *redis.Client
	Metrics *metrics.Metrics
}

func BuildApp(ctx context.Context, cfg *config.Config, log *slog.Logger) (*App, error) {
	dbPool, err := database.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			dbPool.Close()
		}
	}()

	rdb := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisHost + ":" + cfg.RedisPort,
		Password: "",
		Protocol: 3,
	})
	defer func() {
		if err != nil {
			rdb.Close()
		}
	}()

	if err = redisotel.InstrumentTracing(rdb); err != nil {
		return nil, err
	}

	appMetrics := metrics.New()
	if err = appMetrics.RegisterPool(dbPool.Pool); err != nil {
		return nil, err
	}

	loginRateLimiter := ratelimit.New(rdb, cfg.LoginRateLimit, cfg.LoginRateWindow)

	jwtManager := auth.NewJWTManager(cfg.JwtSecret, cfg.JwtExpiry)

	healthHandler := health.New(dbPool.Pool, rdb, log)

	idempotencyQueries := idempotencySqlc.New(dbPool.Pool)
	idempotencyRepository := idempotency.NewIdempotencyRepository(idempotencyQueries)
	idempotencyService := idempotency.NewIdempotencyService(idempotencyRepository)

	userQueries := userSqlc.New(dbPool.Pool)
	userRepository := user.NewUserRepository(userQueries)
	userService := user.NewUserService(userRepository, jwtManager)
	userHandler := user.NewUserHandler(userService, log, jwtManager, loginRateLimiter)

	productQueries := productSqlc.New(dbPool.Pool)
	productRepository := product.NewProductRepository(productQueries)
	productService := product.NewProductService(productRepository)
	productHandler := product.NewProductHandler(productService, log)

	orderQueries := orderSqlc.New(dbPool.Pool)
	orderRepository := order.NewOrderRepository(orderQueries, dbPool.Pool)
	orderService := order.NewOrderService(orderRepository)
	orderHandler := order.NewOrderHandler(orderService, log, jwtManager, idempotencyService)

	paymentQueries := paymentSqlc.New(dbPool.Pool)
	paymentRepository := payment.NewPaymentRepository(paymentQueries, dbPool.Pool)
	paymentService := payment.NewPaymentService(paymentRepository)
	paymentHandler := payment.NewPaymentHandler(paymentService, jwtManager, log)

	router := NewRouter(Handler{
		User:    userHandler,
		Health:  healthHandler,
		Product: productHandler,
		Order:   orderHandler,
		Payment: paymentHandler,
	}, appMetrics)

	return &App{
		Router:  router,
		DB:      dbPool,
		Redis:   rdb,
		Metrics: appMetrics,
	}, nil
}

//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestOrderConcurrency(t *testing.T) {
	env := setupTestEnv(t)

	email := fmt.Sprintf("buyer-%s@example.com", uuid.NewString())
	password := "supersecret123"

	registerBody, _ := json.Marshal(map[string]string{
		"email":    email,
		"password": password,
	})
	registerResp := doRequest(t, env.baseURL, http.MethodPost, "/api/users/register", "", registerBody)
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)

	loginResp := doRequest(t, env.baseURL, http.MethodPost, "/api/users/login", "", registerBody)
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var loginPayload struct {
		Token string `json:"token"`
	}
	decodeData(t, loginResp, &loginPayload)
	require.NotEmpty(t, loginPayload.Token)

	const initialStock = 10
	productBody, _ := json.Marshal(map[string]any{
		"name":        "Concurrency Test Widget",
		"description": "seeded by TestOrderConcurrency",
		"price":       1500,
		"stock":       initialStock,
	})
	productResp := doRequest(t, env.baseURL, http.MethodPost, "/api/products", loginPayload.Token, productBody)
	require.Equal(t, http.StatusCreated, productResp.StatusCode)

	var product struct {
		ID uuid.UUID `json:"id"`
	}
	decodeData(t, productResp, &product)
	require.NotEqual(t, uuid.Nil, product.ID)

	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 200,
	}
	client := &http.Client{Transport: transport}

	const totalRequests = 100
	var (
		wg            sync.WaitGroup
		successCount  atomic.Int32
		conflictCount atomic.Int32
		otherCount    atomic.Int32
	)

	orderBody, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"product_id": product.ID, "quantity": 1},
		},
	})

	for i := 0; i < totalRequests; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			req, err := http.NewRequest(http.MethodPost, env.baseURL+"/api/orders/", bytes.NewReader(orderBody))
			if err != nil {
				otherCount.Add(1)
				return
			}
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+loginPayload.Token)
			req.Header.Set("Idempotency-Key", uuid.NewString())

			resp, err := client.Do(req)
			if err != nil {
				otherCount.Add(1)
				return
			}
			defer resp.Body.Close()

			switch resp.StatusCode {
			case http.StatusCreated:
				successCount.Add(1)
			case http.StatusConflict:
				conflictCount.Add(1)
			default:
				otherCount.Add(1)
			}
		}()
	}
	wg.Wait()

	require.Equal(t, int32(0), otherCount.Load(), "unexpected status codes were returned")
	require.Equal(t, int32(initialStock), successCount.Load(), "expected exactly stock-many orders to succeed")
	require.Equal(t, int32(totalRequests-initialStock), conflictCount.Load(), "expected the remainder to be rejected for insufficient stock")

	var finalStock int
	err := env.pool.QueryRow(t.Context(), "SELECT stock FROM products WHERE id = $1", product.ID).Scan(&finalStock)
	require.NoError(t, err)
	require.Equal(t, 0, finalStock)

	var orderCount int
	err = env.pool.QueryRow(t.Context(), `
		SELECT count(*) FROM orders o
		JOIN order_items oi ON oi.order_id = o.id
		WHERE oi.product_id = $1
	`, product.ID).Scan(&orderCount)
	require.NoError(t, err)
	require.Equal(t, initialStock, orderCount)
}

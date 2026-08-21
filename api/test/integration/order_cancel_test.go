//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// registerAndLogin returns a bearer token for a fresh user.
func registerAndLogin(t *testing.T, baseURL string) string {
	t.Helper()

	credentials, _ := json.Marshal(map[string]string{
		"email":    fmt.Sprintf("buyer-%s@example.com", uuid.NewString()),
		"password": "supersecret123",
	})

	registerResp := doRequest(t, baseURL, http.MethodPost, "/api/users/register", "", credentials)
	require.Equal(t, http.StatusCreated, registerResp.StatusCode)

	loginResp := doRequest(t, baseURL, http.MethodPost, "/api/users/login", "", credentials)
	require.Equal(t, http.StatusOK, loginResp.StatusCode)

	var payload struct {
		Token string `json:"token"`
	}
	decodeData(t, loginResp, &payload)
	require.NotEmpty(t, payload.Token)

	return payload.Token
}

func seedProduct(t *testing.T, baseURL, token string, price, stock int) uuid.UUID {
	t.Helper()

	body, _ := json.Marshal(map[string]any{
		"name":        "Cancellable Widget " + uuid.NewString(),
		"description": "seeded by the cancellation tests",
		"price":       price,
		"stock":       stock,
	})

	resp := doRequest(t, baseURL, http.MethodPost, "/api/products", token, body)
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var created struct {
		ID uuid.UUID `json:"id"`
	}
	decodeData(t, resp, &created)

	return created.ID
}

func placeOrder(t *testing.T, baseURL, token string, productID uuid.UUID, quantity int) uuid.UUID {
	t.Helper()

	body, _ := json.Marshal(map[string]any{
		"items": []map[string]any{{"product_id": productID, "quantity": quantity}},
	})

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/orders/", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Idempotency-Key", uuid.NewString())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var payload struct {
		Order struct {
			ID uuid.UUID `json:"id"`
		} `json:"order"`
	}
	decodeData(t, resp, &payload)

	return payload.Order.ID
}

func productStock(t *testing.T, env testEnv, productID uuid.UUID) int {
	t.Helper()

	var stock int
	require.NoError(t, env.pool.QueryRow(t.Context(),
		"SELECT stock FROM products WHERE id = $1", productID).Scan(&stock))

	return stock
}

// This is the test that was missing while POST /api/orders/{id}/cancel was
// unreachable: the endpoint answered 404 in production and nothing noticed.
func TestCancelOrder_RestoresStockExactlyOnce(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env.baseURL)
	productID := seedProduct(t, env.baseURL, token, 1500, 10)

	orderID := placeOrder(t, env.baseURL, token, productID, 4)
	require.Equal(t, 6, productStock(t, env, productID), "placing the order must reserve stock")

	cancelResp := doRequest(t, env.baseURL, http.MethodPost,
		"/api/orders/"+orderID.String()+"/cancel", token, nil)
	require.Equal(t, http.StatusOK, cancelResp.StatusCode)

	var cancelled struct {
		Status string `json:"status"`
	}
	decodeData(t, cancelResp, &cancelled)
	require.Equal(t, "cancelled", cancelled.Status)
	require.Equal(t, 10, productStock(t, env, productID), "cancelling must return the reserved stock")

	// A retried cancel must be refused rather than crediting the stock twice.
	repeatResp := doRequest(t, env.baseURL, http.MethodPost,
		"/api/orders/"+orderID.String()+"/cancel", token, nil)
	require.Equal(t, http.StatusConflict, repeatResp.StatusCode)
	require.Equal(t, 10, productStock(t, env, productID), "a second cancel must not restore stock again")
}

func TestCancelOrder_UnknownOrder(t *testing.T) {
	env := setupTestEnv(t)
	token := registerAndLogin(t, env.baseURL)

	resp := doRequest(t, env.baseURL, http.MethodPost,
		"/api/orders/"+uuid.NewString()+"/cancel", token, nil)

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
}

// Cancellation is scoped to the owner: another user's order must look like it
// does not exist, and their stock must be untouched.
func TestCancelOrder_OtherUsersOrderIsNotVisible(t *testing.T) {
	env := setupTestEnv(t)

	ownerToken := registerAndLogin(t, env.baseURL)
	productID := seedProduct(t, env.baseURL, ownerToken, 2000, 5)
	orderID := placeOrder(t, env.baseURL, ownerToken, productID, 2)
	require.Equal(t, 3, productStock(t, env, productID))

	intruderToken := registerAndLogin(t, env.baseURL)

	resp := doRequest(t, env.baseURL, http.MethodPost,
		"/api/orders/"+orderID.String()+"/cancel", intruderToken, nil)

	require.Equal(t, http.StatusNotFound, resp.StatusCode)
	require.Equal(t, 3, productStock(t, env, productID), "stock must not move for a rejected cancel")
}

func TestCancelOrder_RequiresAuthentication(t *testing.T) {
	env := setupTestEnv(t)

	resp := doRequest(t, env.baseURL, http.MethodPost,
		"/api/orders/"+uuid.NewString()+"/cancel", "", nil)

	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

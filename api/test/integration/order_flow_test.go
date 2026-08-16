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

type envelope struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
}

func decodeData(t *testing.T, resp *http.Response, out any) {
	t.Helper()

	var env envelope
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&env))
	require.True(t, env.Success, "expected success response, got error: %s", env.Error)
	require.NoError(t, json.Unmarshal(env.Data, out))
}

func TestOrderFlow(t *testing.T) {
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

	productBody, _ := json.Marshal(map[string]any{
		"name":        "Integration Test Widget",
		"description": "seeded by TestOrderFlow",
		"price":       1500,
		"stock":       10,
	})
	productResp := doRequest(t, env.baseURL, http.MethodPost, "/api/products", loginPayload.Token, productBody)
	require.Equal(t, http.StatusCreated, productResp.StatusCode)

	var product struct {
		ID uuid.UUID `json:"id"`
	}
	decodeData(t, productResp, &product)
	require.NotEqual(t, uuid.Nil, product.ID)

	orderBody, _ := json.Marshal(map[string]any{
		"items": []map[string]any{
			{"product_id": product.ID, "quantity": 2},
		},
	})
	orderReq, err := http.NewRequest(http.MethodPost, env.baseURL+"/api/orders/", bytes.NewReader(orderBody))
	require.NoError(t, err)
	orderReq.Header.Set("Content-Type", "application/json")
	orderReq.Header.Set("Authorization", "Bearer "+loginPayload.Token)
	orderReq.Header.Set("Idempotency-Key", uuid.NewString())
	orderResp, err := http.DefaultClient.Do(orderReq)
	require.NoError(t, err)
	t.Cleanup(func() { _ = orderResp.Body.Close() })
	require.Equal(t, http.StatusCreated, orderResp.StatusCode)

	var orderPayload struct {
		Order struct {
			ID     uuid.UUID `json:"id"`
			Status string    `json:"status"`
		} `json:"order"`
	}
	decodeData(t, orderResp, &orderPayload)
	require.NotEqual(t, uuid.Nil, orderPayload.Order.ID)

	payResp := doRequest(t, env.baseURL, http.MethodPost, "/api/orders/"+orderPayload.Order.ID.String()+"/pay", loginPayload.Token, nil)
	require.Equal(t, http.StatusOK, payResp.StatusCode)

	var status string
	err = env.pool.QueryRow(t.Context(), "SELECT status FROM orders WHERE id = $1", orderPayload.Order.ID).Scan(&status)
	require.NoError(t, err)
	require.Equal(t, "paid", status)
}

func doRequest(t *testing.T, baseURL, method, path, token string, body []byte) *http.Response {
	t.Helper()

	var reqBody *bytes.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	} else {
		reqBody = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequest(method, baseURL+path, reqBody)
	require.NoError(t, err)

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = resp.Body.Close()
	})

	return resp
}

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

const (
	testEmail       = "ratelimit@example.com"
	testPassword    = "correct-password"
	testBadPassword = "wrong-password"
)

func registerUser(t *testing.T, baseURL, email, password string) {
	t.Helper()

	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	require.NoError(t, err)

	resp := doRequest(t, baseURL, http.MethodPost, "/api/users/register", "", body)
	defer resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

func attemptLogin(t *testing.T, baseURL, email, password string) *http.Response {
	t.Helper()

	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	require.NoError(t, err)

	return doRequest(t, baseURL, http.MethodPost, "/api/users/login", "", body)
}

// TestLoginRateLimitBlocksAfterLimit adalah AC utama issue #54: percobaan login
// yang ngelewatin batas harus dapat 429, bukan 401.
func TestLoginRateLimitBlocksAfterLimit(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.LoginRateLimit = 3
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	// Tepat sebanyak limit: semua harus nyampe ke handler dan ditolak sebagai
	// kredensial salah (401), bukan kena rate limit.
	for i := 1; i <= 3; i++ {
		resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "percobaan ke-%d harusnya masih diladeni", i)
		resp.Body.Close()
	}

	// Percobaan berikutnya: jatah habis.
	resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)

	var payload struct {
		Success bool   `json:"success"`
		Error   string `json:"error"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.False(t, payload.Success)
	require.NotEmpty(t, payload.Error)
}

// TestLoginRateLimitBlocksEvenWithCorrectPassword mastiin blokirnya kejadian di
// middleware, SEBELUM password dicek. Kalau test ini dapat 200, artinya urutan
// pengecekannya kebalik dan brute force masih bisa nembus.
func TestLoginRateLimitBlocksEvenWithCorrectPassword(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.LoginRateLimit = 2
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	for i := 0; i < 2; i++ {
		resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
		resp.Body.Close()
	}

	resp := attemptLogin(t, env.baseURL, testEmail, testPassword)
	defer resp.Body.Close()
	require.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
}

// TestLoginRateLimitCountsOnlyFailures ngejaga supaya login yang BERHASIL gak
// ikut ngabisin jatah. Kalau yang dihitung semua percobaan, user normal yang
// login 6x sehari bakal ke-lock.
func TestLoginRateLimitCountsOnlyFailures(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.LoginRateLimit = 3
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	for i := 1; i <= 6; i++ {
		resp := attemptLogin(t, env.baseURL, testEmail, testPassword)
		require.Equal(t, http.StatusOK, resp.StatusCode, "login sukses ke-%d harusnya gak kena limit", i)
		resp.Body.Close()
	}
}

// TestLoginRateLimitResetsAfterSuccess: login berhasil harus ngebersihin counter,
// jadi orang yang lupa password lalu akhirnya inget balik dapat jatah penuh.
func TestLoginRateLimitResetsAfterSuccess(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.LoginRateLimit = 3
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	// Habisin 2 dari 3 jatah.
	for i := 0; i < 2; i++ {
		resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		resp.Body.Close()
	}

	okResp := attemptLogin(t, env.baseURL, testEmail, testPassword)
	require.Equal(t, http.StatusOK, okResp.StatusCode)
	okResp.Body.Close()

	// Counter harus balik 0: 3 kegagalan lagi masih diladeni sebagai 401.
	for i := 1; i <= 3; i++ {
		resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "setelah reset, kegagalan ke-%d harusnya masih diladeni", i)
		resp.Body.Close()
	}
}

// TestLoginRateLimitWindowExpires ngebuktiin pemulihan blokirnya beneran jalan
// lewat TTL Redis. Kalau TTL-nya gagal kepasang (misal balik ke pola INCR lalu
// EXPIRE terpisah), IP-nya bakal ke-lock selamanya dan test ini gagal.
func TestLoginRateLimitWindowExpires(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.LoginRateLimit = 2
		cfg.LoginRateWindow = 2 * time.Second
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	for i := 0; i < 2; i++ {
		resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
		resp.Body.Close()
	}

	blocked := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
	require.Equal(t, http.StatusTooManyRequests, blocked.StatusCode)
	blocked.Body.Close()

	time.Sleep(3 * time.Second)

	resp := attemptLogin(t, env.baseURL, testEmail, testPassword)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "setelah window lewat, login harusnya boleh lagi")
}

// TestLoginRateLimitTTLIsSet ngecek langsung ke Redis kalau key-nya punya TTL.
// Test di atas ngecek lewat perilaku; yang ini ngecek penyebabnya, biar kalau
// gagal langsung ketauan masalahnya di TTL, bukan di tempat lain.
func TestLoginRateLimitTTLIsSet(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.LoginRateLimit = 5
		cfg.LoginRateWindow = 10 * time.Minute
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	resp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
	resp.Body.Close()

	keys, err := env.redis.Keys(t.Context(), "rate_limit:login:ip:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1, "satu IP harusnya bikin tepat satu key; kalau lebih, key-nya kemungkinan masih kebawa nomor port")

	ttl, err := env.redis.TTL(t.Context(), keys[0]).Result()
	require.NoError(t, err)
	require.Greater(t, ttl, time.Duration(0), "key rate limit wajib punya TTL, kalau nggak IP-nya ke-lock selamanya")
	require.LessOrEqual(t, ttl, 10*time.Minute)
}

// TestLoginFailsOpenWhenRedisDown adalah AC "kalau Redis gagal, login tetap
// jalan". Redis-nya diarahin ke port mati, bukan dimatiin containernya, supaya
// test-nya deterministik.
func TestLoginFailsOpenWhenRedisDown(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.RedisPort = "1"
	})

	registerUser(t, env.baseURL, testEmail, testPassword)

	resp := attemptLogin(t, env.baseURL, testEmail, testPassword)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode, "Redis mati gak boleh bikin login ikut mati")

	// Kredensial salah tetap ditolak seperti biasa — fail-open cuma matiin
	// rate limiter, bukan matiin autentikasi.
	badResp := attemptLogin(t, env.baseURL, testEmail, testBadPassword)
	defer badResp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, badResp.StatusCode)
}

// TestReadyzIgnoresRedis adalah AC "Redis opsional di /readyz". Redis mati gak
// boleh bikin instance dicabut dari load balancer.
func TestReadyzIgnoresRedis(t *testing.T) {
	env := setupTestEnv(t, func(cfg *config.Config) {
		cfg.RedisPort = "1"
	})

	resp := doRequest(t, env.baseURL, http.MethodGet, "/readyz", "", nil)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Data struct {
			DB    string `json:"db"`
			Redis string `json:"redis"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, "up", payload.Data.DB)
	require.Equal(t, "down", payload.Data.Redis, "status Redis harus dilaporin apa adanya, walaupun gak ngaruh ke status code")
}

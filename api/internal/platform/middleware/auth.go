package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/auth"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/httpx"
	"github.com/AzafaDev/distributed-order-processing-api/internal/platform/ratelimit"
	"github.com/google/uuid"
)

type userIDKey string

const userID userIDKey = "userID"

func Auth(jwtManager *auth.JWTManager, log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			tokenString := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer"))
			userClaims, err := jwtManager.ValidateToken(tokenString, string(jwtManager.Secret))
			if err != nil {
				log.Error("auth middleware", "error", err)
				w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
				httpx.WriteErrorJSON(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}
			ctx := context.WithValue(r.Context(), userID, userClaims.UserID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserIDClaims(ctx context.Context) (uuid.UUID, error) {
	userID, ok := ctx.Value(userID).(uuid.UUID)
	if !ok {
		return uuid.Nil, fmt.Errorf("error in asserting value from context")
	}

	return userID, nil
}

type LoginAllower interface {
	Allow(ctx context.Context, key string) (bool, error)
}

func LoginRateLimiter(rl LoginAllower, log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := httpx.ClientIP(r)

			allowed, err := rl.Allow(r.Context(), ratelimit.LoginKey(ip))
			if err != nil {
				log.Error("login rate limiter: check failed, allowing request", "error", err)
				next.ServeHTTP(w, r)
				return
			}

			if !allowed {
				log.Info("login rate limiter: blocked", "ip", ip)
				httpx.WriteErrorJSON(w, http.StatusTooManyRequests, "too many login attempts, please try again later")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

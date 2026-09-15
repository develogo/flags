package middleware

import (
	"flags/internal/config"
	"flags/internal/models"
	"net/http"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
)

type ipLimiter struct {
	tokens    float64
	lastCheck time.Time
}

type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*ipLimiter
	rate     float64
}

func NewRateLimiter(cfg *config.Config) *RateLimiter {
	rate := float64(cfg.App.RateLimit)
	if rate <= 0 {
		rate = 100
	}
	return &RateLimiter{
		limiters: make(map[string]*ipLimiter),
		rate:     rate,
	}
}

func (rl *RateLimiter) Middleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			ip := c.RealIP()

			rl.mu.Lock()
			lim, exists := rl.limiters[ip]
			if !exists {
				lim = &ipLimiter{tokens: rl.rate, lastCheck: time.Now()}
				rl.limiters[ip] = lim
			}

			now := time.Now()
			elapsed := now.Sub(lim.lastCheck).Seconds()
			lim.tokens += elapsed * rl.rate
			if lim.tokens > rl.rate {
				lim.tokens = rl.rate
			}
			lim.lastCheck = now

			if lim.tokens < 1 {
				rl.mu.Unlock()
				return c.JSON(http.StatusTooManyRequests, models.ErrorResponse{
					Error: "Rate limit exceeded",
				})
			}

			lim.tokens--
			rl.mu.Unlock()

			return next(c)
		}
	}
}

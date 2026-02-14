package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dunamismax/pixelflow/internal/ratelimit"
)

type RateLimiter interface {
	Allow(ctx context.Context, subject string) (ratelimit.Decision, error)
}

func (s *Server) withRateLimit(next http.Handler) http.Handler {
	if s.rateLimiter == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldRateLimit(r) {
			next.ServeHTTP(w, r)
			return
		}

		subject := strings.TrimSpace(r.Header.Get(s.rateLimitUserIDHeader))
		if subject == "" {
			subject = "anonymous"
		}
		subject = subject + ":" + routeLabel(r.URL.Path)

		decision, err := s.rateLimiter.Allow(r.Context(), subject)
		if err != nil {
			s.logger.Printf("rate limiter check failed for subject=%s err=%v", subject, err)
			next.ServeHTTP(w, r)
			return
		}

		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(decision.Remaining, 10))
		if decision.Allowed {
			next.ServeHTTP(w, r)
			return
		}

		retryAfter := int(decision.RetryAfter.Round(time.Second).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
		s.metrics.rateLimitRejected.WithLabelValues(routeLabel(r.URL.Path)).Inc()
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "rate limit exceeded",
		})
	})
}

func shouldRateLimit(r *http.Request) bool {
	if r.Method == http.MethodGet {
		return false
	}
	return strings.HasPrefix(r.URL.Path, "/v1/jobs")
}

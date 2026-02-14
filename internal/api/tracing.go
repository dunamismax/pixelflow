package api

import (
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func (s *Server) withTracing(next http.Handler) http.Handler {
	if s.tracer == nil {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		spanName := r.Method + " " + routeLabel(r.URL.Path)
		ctx, span := s.tracer.Start(r.Context(), spanName, trace.WithSpanKind(trace.SpanKindServer))
		span.SetAttributes(
			attribute.String("http.method", r.Method),
			attribute.String("http.route", routeLabel(r.URL.Path)),
			attribute.String("http.target", r.URL.Path),
		)
		defer span.End()

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

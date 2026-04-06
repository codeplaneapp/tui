package server

import (
	"net/http"

	"github.com/charmbracelet/crush/internal/observability"
)

func (s *Server) loggingHandler(next http.Handler) http.Handler {
	return observability.HTTPServerMiddleware(next)
}

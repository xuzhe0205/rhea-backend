package httpapi

import (
	"net/http"
)

type Server struct {
	mux *http.ServeMux
}

func NewServer() *Server {
	return &Server{mux: http.NewServeMux()}
}

func (s *Server) Handle(pattern string, h http.Handler) {
	s.mux.Handle(pattern, h)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

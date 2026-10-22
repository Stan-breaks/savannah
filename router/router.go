package router

import (
	"net/http"
)

type Router struct {
	mux *http.ServeMux
}

func NewRouter() *Router {
	r := &Router{
		mux: http.NewServeMux(),
	}
	r.setupRoutes()
	return r
}

func (r *Router) setupRoutes() {
	// API routes are registered here as they are implemented.
}

func (r *Router) Handler() http.Handler {
	return r.mux
}

package router

import (
	"net/http"
	"savannah/database"
	"savannah/router/apis"
)

type Router struct {
	queries   *database.Queries
	jwtSecret []byte
	mux       *http.ServeMux
}

func NewRouter(queries *database.Queries) *Router {
	r := &Router{
		queries:   queries,
		mux:       http.NewServeMux(),
	}
	r.setupRoutes()
	return r
}

func (r *Router) setupRoutes() {

	userHandler := apis.NewUserApi(r.queries)
	r.mux.HandleFunc("/api/me",)
}

func (r *Router) Handler() http.Handler {
	return r.mux
}

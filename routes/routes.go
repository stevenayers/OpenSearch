package routes

import (
	"clamber/handlers"
	"clamber/logger"
	"github.com/gorilla/mux"
	"net/http"
)

type (
	Routes []Route

	Route struct {
		Name        string
		Method      string
		Pattern     string
		HandlerFunc http.HandlerFunc
		Queries     []string
	}
)

var DefinedRoutes = Routes{
	Route{
		"Initiate",
		"GET",
		"/search",
		handlers.Search,
		[]string{
			"url", "{url}",
			"depth", "{depth}"},
	},
}

func NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range DefinedRoutes {
		handler := logging.Logger(route.HandlerFunc)
		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler).Queries(route.Queries...)
	}

	return router
}

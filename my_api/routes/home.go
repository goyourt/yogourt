package routes

import (
	"net/http"
	"myApi/handlers" // Importation du handler
)

//Définit la route et appelle le handler associé
func homeRoute(mux *http.ServeMux) {
	mux.HandleFunc("/home", handlers.HelloHandler)
}

package main

import (
	"fmt"
	"net/http"
)

func main() {
	mux := http.NewServeMux()

	// Démarre le serveur HTTP
	port := "8080"
	fmt.Printf("Serveur démarré sur le port %s\n", port)
	err := http.ListenAndServe(":"+port, mux)
	if err != nil {
		fmt.Println("Erreur lors du démarrage du serveur :", err)
	}
}

package handlers

import (
	"fmt"
	"net/http"
)

//Gère les requêtes pour la route associée
func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
		return
	}

	_, err := w.Write([]byte(`{"message": "Bienvenue sur la route hello !"}`))
	if err != nil {
		fmt.Printf("Erreur lors de l'écriture de la réponse: %v \n", err)
	}
}

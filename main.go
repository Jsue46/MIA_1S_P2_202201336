package main

import (
	"Proyecto/Analyzer"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type CommandRequest struct {
	Command string `json:"command"`
}

func executeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	var request CommandRequest
	err := json.NewDecoder(r.Body).Decode(&request)
	if err != nil {
		http.Error(w, "Error al decodificar JSON", http.StatusBadRequest)
		return
	}

	var results []string
	lines := strings.Split(request.Command, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Omitir completamente comentarios y líneas vacías
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		comando, parametros := Analyzer.GetEntrada(line)
		result := Analyzer.AnalizadorComandos(comando, parametros)
		results = append(results, result)
	}

	response := map[string]string{"output": strings.Join(results, "\n")}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func main() {
	router := mux.NewRouter()
	router.HandleFunc("/execute", executeHandler).Methods("POST")

	cors := handlers.CORS(
		handlers.AllowedOrigins([]string{"*"}),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type"}),
	)

	fmt.Println("SERVIDOR CORRIENDO EN http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", cors(router)))
}

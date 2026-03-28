package middleware

import (
	"encoding/json"
	"log"
	"net/http"
)

func WriteJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func WriteJSONInternalError(w http.ResponseWriter, err error, context string) {
	log.Printf("%s: %v", context, err)
	WriteJSONError(w, http.StatusInternalServerError, "internal server error")
}

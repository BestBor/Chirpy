package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type apiConfig struct {
	fileServerHits atomic.Int32
}

func main() {
	cfg := &apiConfig{}
	mux := http.NewServeMux()

	// Endpoints básicos
	mux.HandleFunc("GET /healthz", healthHandler)
	mux.HandleFunc("GET /metrics", cfg.metricsHandler)
	mux.HandleFunc("POST /reset", cfg.resetHandler)

	// File server con middleware de métricas
	fileServer := http.FileServer(http.Dir("."))
	appHandler := http.StripPrefix("/app/", fileServer)

	// Aplicamos middleware solo aquí
	mux.Handle("/app/", cfg.middlewareMetricsInc(appHandler))

	fmt.Println("Servidor escuchando en :8080")
	http.ListenAndServe(":8080", mux)
}

// Handlers

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	hits := cfg.fileServerHits.Load()
	fmt.Fprintf(w, "Hits: %d\n", hits)
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	cfg.fileServerHits.Store(0)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset\n"))
}

// Middleware

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileServerHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

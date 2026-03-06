package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync/atomic"

	"github.com/BestBor/Chirpy/internal/database"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileServerHits atomic.Int32
	db             *database.Queries
	platform       string
	secret         string
	polkaKey       string
}

func main() {
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL not set")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal("DB pool not prepared")
	}
	if err := db.Ping(); err != nil {
		log.Fatal("Database not reachable")
	}

	cfg := &apiConfig{
		db:       database.New(db),
		platform: os.Getenv("PLATFORM"),
		secret:   os.Getenv("SECRET"),
		polkaKey: os.Getenv("POLKA_KEY"),
	}
	mux := http.NewServeMux()

	defer db.Close()

	// Endpoints
	// /api/
	mux.HandleFunc("GET /api/healthz", healthHandler)
	// Refresh
	mux.HandleFunc("POST /api/refresh", cfg.validateRToken)
	mux.HandleFunc("POST /api/revoke", cfg.revokeRToken)
	// Users
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)
	mux.HandleFunc("PUT /api/users", cfg.updateUserEmailNPass)
	// Session
	mux.HandleFunc("POST /api/login", cfg.userLogin)
	// Chirps
	mux.HandleFunc("GET /api/chirps", cfg.getAllChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirpHandler)
	mux.HandleFunc("POST /api/chirps", cfg.createChirpHandler)
	mux.HandleFunc("DELETE /api/chirps/{chirpID}", cfg.deleteChirpHandler)
	// mux.HandleFunc("POST /api/validate_chirp", validationHandler)
	// Polka (payments)
	mux.HandleFunc("POST /api/polka/webhooks", cfg.updateUserChirpyRed)
	// /admin/
	mux.HandleFunc("GET /admin/metrics", cfg.metricsHandler)
	mux.HandleFunc("POST /admin/reset", cfg.resetHandler)

	// /app/
	// File server - middleware
	fileServer := http.FileServer(http.Dir("."))
	appHandler := http.StripPrefix("/app/", fileServer)

	// Applied middleware
	mux.Handle("/app/", cfg.middlewareMetricsInc(appHandler))

	fmt.Println("Server listening on PORT :8080")
	http.ListenAndServe(":8080", mux)
}

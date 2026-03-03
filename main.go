package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/BestBor/Chirpy/internal/auth"
	"github.com/BestBor/Chirpy/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	fileServerHits atomic.Int32
	db             *database.Queries
	platform       string
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
	}
	mux := http.NewServeMux()

	defer db.Close()

	// Endpoints
	// /api/
	mux.HandleFunc("GET /api/healthz", healthHandler)
	// Users
	mux.HandleFunc("POST /api/users", cfg.createUserHandler)
	// Session
	mux.HandleFunc("POST /api/login", cfg.userLogin)
	// Chirps
	mux.HandleFunc("GET /api/chirps", cfg.getAllChirpsHandler)
	mux.HandleFunc("GET /api/chirps/{chirpID}", cfg.getChirpHandler)
	mux.HandleFunc("POST /api/chirps", cfg.createChirpHandler)
	// mux.HandleFunc("POST /api/validate_chirp", validationHandler)
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

// Types

type newChirpRequest struct {
	Body   string    `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}

type userRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type User struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

// Handlers
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK\n"))
}

func (cfg *apiConfig) metricsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	hits := cfg.fileServerHits.Load()
	genBody := fmt.Sprintf(`
		<html>
		<body>
			<h1>Welcome, Chirpy Admin</h1>
			<p>Chirpy has been visited %d times!</p>
		</body>
		</html>`, hits)
	w.Write([]byte(genBody))
	// shorter option yet a little confusing
	// fmt.Fprintf(w, "Hits: %d\n", hits)
}

func (cfg *apiConfig) resetHandler(w http.ResponseWriter, r *http.Request) {
	if cfg.platform != "dev" {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	cfg.fileServerHits.Store(0)
	cfg.db.DeleteAllUsers(r.Context())
	cfg.db.DeleteAllChirps(r.Context())
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Hits reset\n"))
}

// Users CRUD Handlers
func (cfg *apiConfig) createUserHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req userRequest

	dec := json.NewDecoder(r.Body)

	if err := dec.Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	hashedPass, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error hashing password")
		return
	}

	dbUser, err := cfg.db.CreateUser(r.Context(), database.CreateUserParams{
		Email:          req.Email,
		HashedPassword: hashedPass,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem creating user")
		return
	}

	respondWithJSON(w, http.StatusCreated, User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	})

}

// Users session
func (cfg *apiConfig) userLogin(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req userRequest

	dec := json.NewDecoder(r.Body)

	if err := dec.Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	dbUser, err := cfg.db.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}
	if ok, err := auth.CheckPasswordHash(req.Password, dbUser.HashedPassword); err != nil || !ok {
		respondWithError(w, http.StatusUnauthorized, "Incorrect email or password")
		return
	}

	respondWithJSON(w, http.StatusOK, User{
		ID:        dbUser.ID,
		CreatedAt: dbUser.CreatedAt,
		UpdatedAt: dbUser.UpdatedAt,
		Email:     dbUser.Email,
	})

}

// Chirps CRUD Handlers

func (cfg *apiConfig) getAllChirpsHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	dbChirps, err := cfg.db.GetAllChirpsByCreatedAt(r.Context())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting chirps")
		return
	}
	var res []Chirp

	for _, c := range dbChirps {
		res = append(res, Chirp{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserID:    c.UserID,
		})
	}

	respondWithJSON(w, http.StatusOK, res)
}

func (cfg *apiConfig) getChirpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	chirpIDStr := r.PathValue("chirpID")
	fmt.Println("Looking for:", chirpIDStr)

	chirpID, err := uuid.Parse(chirpIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid chirp id")
		return
	}
	dbChirp, err := cfg.db.GetChirp(r.Context(), chirpID)
	if errors.Is(err, sql.ErrNoRows) {
		respondWithError(w, http.StatusNotFound, "chirp not found")
		return
	}
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error getting chirp")
		return
	}

	respondWithJSON(w, http.StatusOK, Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	})

}

func (cfg *apiConfig) createChirpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req newChirpRequest

	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	validate := func(text string) error {
		if len(text) > 140 {
			return fmt.Errorf("Chirp is too long")
		}
		return nil
	}

	if err := validate(req.Body); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	profanityCheck := func(text string) string {
		profanitiesMap := map[string]struct{}{
			"kerfuffle": {},
			"sharbert":  {},
			"fornax":    {},
		}
		parts := strings.Split(text, " ")
		for i, word := range parts {
			if _, exists := profanitiesMap[strings.ToLower(word)]; exists {
				parts[i] = "****"
			}
		}
		return strings.Join(parts, " ")
	}

	dbChirp, err := cfg.db.CreateChirp(r.Context(), database.CreateChirpParams{
		Body:   profanityCheck(req.Body),
		UserID: req.UserID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem creating chirp")
		return
	}

	fmt.Println("POST returning chirp ID:", dbChirp.ID)
	fmt.Println("POST returning user ID:", dbChirp.UserID)

	respondWithJSON(w, http.StatusCreated, Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	})

}

// Middleware

func (cfg *apiConfig) middlewareMetricsInc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg.fileServerHits.Add(1)
		next.ServeHTTP(w, r)
	})
}

// Helpers

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	data, err := json.Marshal(payload)

	if err != nil {
		log.Printf("Error marshalling JSON: %s", err)
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(data)
}

func respondWithError(w http.ResponseWriter, code int, msg string) {
	type errorResponse struct {
		Error string `json:"error"`
	}

	respondWithJSON(w, code, errorResponse{
		Error: msg,
	})
}

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

// Types

type newChirpRequest struct {
	Body   string    `json:"body"`
	UserID uuid.UUID `json:"user_id"`
}

type userRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// ExpiresInSeconds *int   `json:"expires_in_seconds,omitempty"`
}

type User struct {
	ID           uuid.UUID `json:"id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Email        string    `json:"email"`
	Token        string    `json:"token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	IsChirpyRed  bool      `json:"is_chirpy_red"`
}

type Chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserID    uuid.UUID `json:"user_id"`
}

type PolkaRequest struct {
	Event string `json:"event"`
	Data  struct {
		UserID string `json:"user_id"`
	} `json:"data"`
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

func (cfg *apiConfig) validateRToken(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	dbRT, err := cfg.db.GetRToken(r.Context(), token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "server error")
		return
	}
	if dbRT.ExpiresAt.Before(time.Now()) || dbRT.RevokedAt.Valid {
		respondWithError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	newJWT, err := auth.MakeJWT(dbRT.UserID, cfg.secret, 1*time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem creating token")
		return
	}
	respondWithJSON(w, http.StatusOK, struct {
		Token string `json:"token"`
	}{Token: newJWT})
}

func (cfg *apiConfig) revokeRToken(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	bearer, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "error retrieving token from request")
		return
	}
	rToken, err := cfg.db.GetRToken(r.Context(), bearer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusUnauthorized, "error retrieving token from database")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "server error")
		return
	}
	cfg.db.RevokeRToken(r.Context(), rToken.Token)
	respondWithJSON(w, http.StatusNoContent, nil)

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
		ID:          dbUser.ID,
		CreatedAt:   dbUser.CreatedAt,
		UpdatedAt:   dbUser.UpdatedAt,
		Email:       dbUser.Email,
		IsChirpyRed: dbUser.IsChirpyRed,
	})

}

func (cfg *apiConfig) updateUserEmailNPass(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	// Get Bearer token
	accessToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Validate JWT
	userID, err := auth.ValidateJWT(accessToken, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Decode body
	var req userRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	hPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "server error")
		return
	}
	// update user on db
	updatedUser, err := cfg.db.UpdateCredentials(r.Context(), database.UpdateCredentialsParams{
		ID:             userID,
		Email:          req.Email,
		HashedPassword: hPassword,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to update user")
		return
	}

	respondWithJSON(w, http.StatusOK, User{
		ID:        updatedUser.ID,
		CreatedAt: updatedUser.CreatedAt,
		UpdatedAt: updatedUser.UpdatedAt,
		Email:     updatedUser.Email,
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

	// expires := 3600
	// if req.ExpiresInSeconds != nil {
	// 	v := *req.ExpiresInSeconds
	// 	if v > 0 && v < 3600 {
	// 		expires = v
	// 	}
	// }

	token, err := auth.MakeJWT(dbUser.ID, cfg.secret, time.Duration(1)*time.Hour)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem creating token")
		return
	}

	refreshToken := auth.MakeRefreshToken()
	_, err = cfg.db.CreateRToken(r.Context(), database.CreateRTokenParams{
		Token:     refreshToken,
		UserID:    dbUser.ID,
		ExpiresAt: time.Now().Add(time.Hour * 24 * 60), // TTL: 60 days
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem creating refresh token")
		return
	}

	respondWithJSON(w, http.StatusOK, User{
		ID:           dbUser.ID,
		CreatedAt:    dbUser.CreatedAt,
		UpdatedAt:    dbUser.UpdatedAt,
		Email:        dbUser.Email,
		Token:        token,
		RefreshToken: refreshToken,
		IsChirpyRed:  dbUser.IsChirpyRed,
	})

}

// Chirps CRUD Handlers

func (cfg *apiConfig) getAllChirpsHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	authorIdQueryParam := r.URL.Query().Get("author_id")
	if authorIdQueryParam == "" {
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
	} else {
		parseduuid, err := uuid.Parse(authorIdQueryParam)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "unable to retrieve chirps")
			return
		}
		dbChirpsById, err := cfg.db.GetAllChirpsByAuthorId(r.Context(), parseduuid)
		if err != nil {
			respondWithError(w, http.StatusInternalServerError, "unable to retrieve chirps")
			return
		}
		var res []Chirp
		for _, c := range dbChirpsById {
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

}

func (cfg *apiConfig) getChirpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	chirpIDStr := r.PathValue("chirpID")

	chirpID, err := uuid.Parse(chirpIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid chirp id")
		return
	}
	dbChirp, err := cfg.db.GetChirpById(r.Context(), chirpID)
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

	token, err := auth.GetBearerToken(r.Header)

	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "missing or invalid token")
		return
	}

	var req newChirpRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Something went wrong")
		return
	}

	if len(req.Body) > 140 {
		respondWithError(w, http.StatusBadRequest, "chirp is too long")
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
		UserID: userID,
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "problem creating chirp")
		return
	}

	respondWithJSON(w, http.StatusCreated, Chirp{
		ID:        dbChirp.ID,
		CreatedAt: dbChirp.CreatedAt,
		UpdatedAt: dbChirp.UpdatedAt,
		Body:      dbChirp.Body,
		UserID:    dbChirp.UserID,
	})

}

func (cfg *apiConfig) deleteChirpHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	// validate path
	chirpIDStr := r.PathValue("chirpID")
	chirpID, err := uuid.Parse(chirpIDStr)
	if err != nil {
		respondWithError(w, http.StatusNotFound, "chirp not found")
		return
	}

	// validate header
	accessToken, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	// Validate JWT
	userID, err := auth.ValidateJWT(accessToken, cfg.secret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	chirp, err := cfg.db.GetChirpById(r.Context(), chirpID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			respondWithError(w, http.StatusNotFound, "chirp not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "unable to fetch chirp")
		return
	}

	if chirp.UserID != userID {
		respondWithError(w, http.StatusForbidden, "unauthorized to delete this chirp")
		return
	}

	err = cfg.db.DeleteChirpById(r.Context(), chirpID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to delete chirp")
		return
	}

	respondWithJSON(w, http.StatusNoContent, nil)

}

// Chirpy Red Handlers - Webhooks
func (cfg *apiConfig) updateUserChirpyRed(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	apiKey, err := auth.GetAPIKey(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, err.Error())
		return
	}

	if apiKey != cfg.polkaKey {
		respondWithError(w, http.StatusUnauthorized, "")
	}

	var req PolkaRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "server error")
		return
	}

	if req.Event != "user.upgraded" {
		respondWithJSON(w, http.StatusNoContent, nil)
		return
	}

	userID, err := uuid.Parse(req.Data.UserID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "invalid format")
		return
	}

	if _, err := cfg.db.UpdateUserToRedById(r.Context(), userID); err != nil {
		respondWithError(w, http.StatusNotFound, "user not found")
		return
	}
	respondWithJSON(w, http.StatusNoContent, nil)

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

	if code == http.StatusNoContent {
		w.WriteHeader(code)
		return
	}

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

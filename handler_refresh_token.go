package main

import (
	"database/sql"
	"errors"
	"net/http"
	"time"

	"github.com/BestBor/Chirpy/internal/auth"
)

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

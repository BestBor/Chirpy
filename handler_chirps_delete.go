package main

import (
	"database/sql"
	"errors"
	"net/http"

	"github.com/BestBor/Chirpy/internal/auth"
	"github.com/google/uuid"
)

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

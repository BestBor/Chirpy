package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/BestBor/Chirpy/internal/auth"
	"github.com/BestBor/Chirpy/internal/database"
)

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

package main

import (
	"encoding/json"
	"net/http"

	"github.com/BestBor/Chirpy/internal/auth"
	"github.com/google/uuid"
)

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

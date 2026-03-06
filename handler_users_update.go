package main

import (
	"encoding/json"
	"net/http"

	"github.com/BestBor/Chirpy/internal/auth"
	"github.com/BestBor/Chirpy/internal/database"
)

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

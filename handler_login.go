package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/BestBor/Chirpy/internal/auth"
	"github.com/BestBor/Chirpy/internal/database"
)

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

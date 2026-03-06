package main

import (
	"database/sql"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/BestBor/Chirpy/internal/database"
	"github.com/google/uuid"
)

func (cfg *apiConfig) getAllChirpsHandler(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	authorIDParam := r.URL.Query().Get("author_id")
	sortParam := r.URL.Query().Get("sort")

	var dbChirps []database.Chirp
	var err error

	if authorIDParam != "" {
		parsedUUID, err := uuid.Parse(authorIDParam)
		if err != nil {
			respondWithError(w, http.StatusBadRequest, "invalid author_id")
			return
		}

		dbChirps, err = cfg.db.GetAllChirpsByAuthorId(r.Context(), parsedUUID)
	} else {
		dbChirps, err = cfg.db.GetAllChirpsByCreatedAt(r.Context())
	}

	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "unable to retrieve chirps")
		return
	}

	res := make([]Chirp, 0, len(dbChirps))

	for _, c := range dbChirps {
		res = append(res, Chirp{
			ID:        c.ID,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
			Body:      c.Body,
			UserID:    c.UserID,
		})
	}

	if strings.ToLower(sortParam) == "desc" {
		sort.Slice(res, func(i, j int) bool {
			return res[i].CreatedAt.After(res[j].CreatedAt)
		})
	}

	respondWithJSON(w, http.StatusOK, res)
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

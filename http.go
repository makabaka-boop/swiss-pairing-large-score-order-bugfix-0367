package pairing

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
)

// Wire colour: the public API accepts colour spellings rather than the
// internal integer enum.
type gameDTO struct {
	Round      int    `json:"round"`
	OpponentID string `json:"opponentId"`
	Color      string `json:"color"`
}

type requestDTO struct {
	Players []Player             `json:"players"`
	Games   map[string][]gameDTO `json:"games"`
	Byes    map[string][]int     `json:"byes"`
}

// NewHandler exposes the service over HTTP.
//
//	POST /pairings/next-round
//
// Malformed input yields HTTP 400 and a body of
//
//	{"status":"INVALID_HISTORY","error":"..."}
//
// A valid input for which no complete plan exists yields HTTP 422 and
//
//	{"status":"NO_PAIRING"}
//
// Success yields HTTP 200 with the serialized Plan.
func NewHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/pairings/next-round", handleNextRound)
	return mux
}

func handleNextRound(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "use POST")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "BAD_REQUEST", "cannot read body: "+err.Error())
		return
	}
	var dto requestDTO
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_HISTORY", "invalid JSON: "+err.Error())
		return
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "INVALID_HISTORY", "unexpected trailing JSON value")
		return
	}

	req := Request{
		Players: dto.Players,
		Games:   make(map[string][]Game, len(dto.Games)),
		Byes:    dto.Byes,
	}
	for id, list := range dto.Games {
		games := make([]Game, 0, len(list))
		for _, g := range list {
			c, err := parseColor(g.Color)
			if err != nil {
				writeError(w, http.StatusBadRequest, "INVALID_HISTORY",
					"bad colour for player "+id+" round "+strconv.Itoa(g.Round)+": "+err.Error())
				return
			}
			games = append(games, Game{Round: g.Round, OpponentID: g.OpponentID, Color: c})
		}
		req.Games[id] = games
	}

	plan, err := NextRound(&req)
	if err != nil {
		if errors.Is(err, ErrNoPairing) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"status": "NO_PAIRING"})
			return
		}
		if IsInvalidHistory(err) {
			writeError(w, http.StatusBadRequest, "INVALID_HISTORY", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, plan)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"status": code, "error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Package pairing implements the next-round orchestration for a Swiss-style
// tournament of 4–12 players.
//
// The input gives each player's current score in half points, every game the
// player has already played (round, opponent, colour) and the rounds in which
// the player had a bye. The produced plan pairs every player that does not
// receive a bye, assigns first/second colours and never delivers a partial
// schedule: when no complete legal plan exists the sentinel error ErrNoPairing
// is returned.
package pairing

import (
	"errors"
	"fmt"
)

// Color is the colour/order a player had, or will have, in a game.
// The zero value is intentionally invalid so missing data is rejected.
type Color int

const (
	ColorInvalid Color = iota
	// ColorFirst means the player moves first (the "white" side).
	ColorFirst
	// ColorSecond means the player moves second (the "black" side).
	ColorSecond
)

// String renders a colour as F / S, the tokens used in the canonical ordering.
func (c Color) String() string {
	switch c {
	case ColorFirst:
		return "F"
	case ColorSecond:
		return "S"
	default:
		return "?"
	}
}

// Player is one tournament participant.
type Player struct {
	// ID is the public, globally comparable player identifier.
	ID string `json:"id"`
	// Score is the current score measured in half points (1 = win, 0.5
	// becomes 1, etc.), so it must be a non-negative integer.
	Score int `json:"score"`
}

// Game is a single game from a player's own history.
type Game struct {
	// Round is the 1-based round number in which the game happened.
	Round int `json:"round"`
	// OpponentID identifies the opponent.
	OpponentID string `json:"opponentId"`
	// Color is the colour the reporting player used in this game.
	Color Color `json:"color"`
}

// Request is the input of NextRound.
type Request struct {
	// Players holds every participant; IDs must be unique.
	Players []Player `json:"players"`
	// Games maps a player ID to the games that player has already played.
	// The two sides of every game must describe each other consistently.
	Games map[string][]Game `json:"games"`
	// Byes maps a player ID to the rounds in which that player had a bye.
	Byes map[string][]int `json:"byes"`
}

// Pair is one game of the next round. FirstID always moves first.
type Pair struct {
	FirstID  string `json:"firstId"`
	SecondID string `json:"secondId"`
}

// Plan is a complete next-round schedule.
type Plan struct {
	// Round is the round number the plan is for (highest known round + 1).
	Round int `json:"round"`
	// Pairs are sorted lexicographically by the pair's sorted ID pair, i.e.
	// by min(first, second) and then max(first, second).
	Pairs []Pair `json:"pairs"`
	// ByeID is the player with the bye for an odd head count; empty for an
	// even head count where nobody has a bye.
	ByeID string `json:"byeId,omitempty"`
}

// ErrNoPairing is returned when the input is valid but no complete legal
// plan exists. Its error text is the protocol token NO_PAIRING.
var ErrNoPairing = errors.New("NO_PAIRING")

// InvalidHistoryError describes a malformed request. Use errors.As to
// distinguish bad input from a legitimate "no plan" answer.
type InvalidHistoryError struct{ msg string }

func (e *InvalidHistoryError) Error() string { return e.msg }

func invalidf(format string, args ...any) error {
	return &InvalidHistoryError{msg: "invalid history: " + fmt.Sprintf(format, args...)}
}

// IsInvalidHistory reports whether err (or any error in its chain) is an
// InvalidHistoryError.
func IsInvalidHistory(err error) bool {
	var target *InvalidHistoryError
	return errors.As(err, &target)
}

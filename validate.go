package pairing

import (
	"fmt"
	"sort"
	"strings"
)

// normalized holds a validated request in a compact, index-friendly shape.
type normalized struct {
	ids    []string // sorted player IDs
	score  []int    // scores indexed by sorted position
	first  []int    // historical first-colour counts
	second []int    // historical second-colour counts
	byes   [][]int  // rounds with a bye, per player
	// played reports whether the players at the two sorted positions have
	// already met in any earlier round.
	played [][]bool
	round  int // next round number
}

// normalize validates req and converts it into the internal representation.
// Any malformed input produces an InvalidHistoryError.
func normalize(req *Request) (*normalized, error) {
	if req == nil {
		return nil, invalidf("missing request body")
	}
	n := len(req.Players)
	if n < 4 || n > 12 {
		return nil, invalidf("player count must be between 4 and 12, got %d", n)
	}

	ids := make([]string, n)
	score := make([]int, n)
	index := make(map[string]int, n)
	for i, p := range req.Players {
		if p.ID == "" {
			return nil, invalidf("player #%d has an empty ID", i)
		}
		if _, dup := index[p.ID]; dup {
			return nil, invalidf("duplicate player ID %q", p.ID)
		}
		if p.Score < 0 {
			return nil, invalidf("player %q has a negative score %d", p.ID, p.Score)
		}
		index[p.ID] = i
		ids[i] = p.ID
		score[i] = p.Score
	}

	first := make([]int, n)
	second := make([]int, n)
	gamesAt := make([][]Game, n)
	byesAt := make([][]int, n)
	maxRound := 0

	// ---- per-player sanity: no duplicate appearances in one round, rounds
	// positive, known opponents, valid colours, referenced players exist.
	validatePlayer := func(id string, byes []int) (int, error) {
		i := index[id]
		seen := make(map[int]Color)
		localMax := 0
		for _, g := range gamesAt[i] {
			if g.Round <= 0 {
				return 0, invalidf("player %q has a game in non-positive round %d", id, g.Round)
			}
			if prior, ok := seen[g.Round]; ok {
				return 0, invalidf("player %q appears twice in round %d (%s and %s games)",
					id, g.Round, prior, g.Color)
			}
			seen[g.Round] = g.Color
			if g.OpponentID == "" {
				return 0, invalidf("player %q has a game in round %d without an opponent", id, g.Round)
			}
			if _, ok := index[g.OpponentID]; !ok {
				return 0, invalidf("player %q played unknown opponent %q in round %d",
					id, g.OpponentID, g.Round)
			}
			if g.OpponentID == id {
				return 0, invalidf("player %q is recorded as playing themselves in round %d", id, g.Round)
			}
			switch g.Color {
			case ColorFirst:
				first[i]++
			case ColorSecond:
				second[i]++
			default:
				return 0, invalidf("player %q has an invalid colour in round %d", id, g.Round)
			}
			if g.Round > localMax {
				localMax = g.Round
			}
		}

		byeRounds := make(map[int]bool, len(byes))
		for _, r := range byes {
			if r <= 0 {
				return 0, invalidf("player %q has a bye in non-positive round %d", id, r)
			}
			if byeRounds[r] {
				return 0, invalidf("player %q is listed with a bye in round %d twice", id, r)
			}
			byeRounds[r] = true
			if _, plays := seen[r]; plays {
				return 0, invalidf("player %q both plays and has a bye in round %d", id, r)
			}
			if r > localMax {
				localMax = r
			}
		}
		byesAt[i] = append([]int(nil), byes...)
		return localMax, nil
	}

	for id, list := range req.Games {
		i, ok := index[id]
		if !ok {
			return nil, invalidf("game history references unknown player %q", id)
		}
		gamesAt[i] = list
	}
	for id := range req.Byes {
		if _, ok := index[id]; !ok {
			return nil, invalidf("bye history references unknown player %q", id)
		}
	}
	for _, id := range ids {
		localMax, err := validatePlayer(id, req.Byes[id])
		if err != nil {
			return nil, err
		}
		if localMax > maxRound {
			maxRound = localMax
		}
	}

	// ---- cross-player consistency: each game must be mirrored by the
	// opponent, in the same round, with the opposite colour.
	for i, id := range ids {
		for _, g := range gamesAt[i] {
			j := index[g.OpponentID]
			reverse := findGame(gamesAt[j], g.Round, id)
			if reverse == nil {
				return nil, invalidf("player %q reports a game against %q in round %d, but %q has no matching record",
					id, g.OpponentID, g.Round, g.OpponentID)
			}
			if reverse.OpponentID != id {
				return nil, invalidf("player %q round %d record points at %q instead of %q",
					g.OpponentID, g.Round, reverse.OpponentID, id)
			}
			if reverse.Color != opposite(g.Color) {
				return nil, invalidf("players %q and %q disagree on colours in round %d",
					id, g.OpponentID, g.Round)
			}
		}
	}

	// ---- re-order everything into ID-sorted positions.
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return ids[order[a]] < ids[order[b]] })

	sorted := &normalized{
		ids:    make([]string, n),
		score:  make([]int, n),
		first:  make([]int, n),
		second: make([]int, n),
		byes:   make([][]int, n),
		played: make([][]bool, n),
		round:  maxRound + 1,
	}
	for i := range sorted.played {
		sorted.played[i] = make([]bool, n)
	}
	newPos := make([]int, n)
	for pos, old := range order {
		sorted.ids[pos] = ids[old]
		sorted.score[pos] = score[old]
		sorted.first[pos] = first[old]
		sorted.second[pos] = second[old]
		sorted.byes[pos] = byesAt[old]
		newPos[old] = pos
	}
	for oldI, list := range gamesAt {
		for _, g := range list {
			sorted.played[newPos[oldI]][newPos[index[g.OpponentID]]] = true
		}
	}
	return sorted, nil
}

func findGame(list []Game, round int, opponent string) *Game {
	for i := range list {
		if list[i].Round == round && list[i].OpponentID == opponent {
			return &list[i]
		}
	}
	return nil
}

func opposite(c Color) Color {
	switch c {
	case ColorFirst:
		return ColorSecond
	case ColorSecond:
		return ColorFirst
	default:
		return ColorInvalid
	}
}

// parseColor accepts the colour spellings used on the wire. Matching is
// case-insensitive and tolerates common aliases.
func parseColor(s string) (Color, error) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "F", "FIRST", "W", "WHITE":
		return ColorFirst, nil
	case "S", "SECOND", "B", "BLACK":
		return ColorSecond, nil
	default:
		return ColorInvalid, fmt.Errorf("unknown color %q", s)
	}
}

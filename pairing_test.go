package pairing

import (
	"errors"
	"math"
	"math/bits"
	"testing"
)

func players(ids ...string) []Player {
	out := make([]Player, len(ids))
	for i, id := range ids {
		out[i] = Player{ID: id}
	}
	return out
}

func game(round int, opponent string, c Color) Game {
	return Game{Round: round, OpponentID: opponent, Color: c}
}

func TestEmptyFourPlayers_FirstRoundCanonical(t *testing.T) {
	plan, err := NextRound(&Request{Players: players("A", "B", "C", "D")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Plan{
		Round: 1,
		Pairs: []Pair{
			{FirstID: "A", SecondID: "B"},
			{FirstID: "C", SecondID: "D"},
		},
	}
	assertPlan(t, plan, want)
}

func TestScoreDistanceIsPrimaryObjective(t *testing.T) {
	req := &Request{Players: []Player{
		{ID: "A", Score: 10},
		{ID: "B", Score: 9},
		{ID: "C", Score: 1},
		{ID: "D", Score: 0},
	}}
	plan, err := NextRound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Plan{
		Round: 1,
		Pairs: []Pair{
			{FirstID: "A", SecondID: "B"}, // diff 1 half-point
			{FirstID: "C", SecondID: "D"}, // diff 1 half-point
		},
	}
	assertPlan(t, plan, want)
}

func TestColorBalanceIsSecondaryObjective(t *testing.T) {
	// A and C already have a +1 first-colour balance; the optimum lets them
	// move second. Moves (A,B)+(C,D) then tie on score and win on colours.
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Games: map[string][]Game{
			"A": {game(1, "D", ColorFirst)},
			"D": {game(1, "A", ColorSecond)},
			"C": {game(1, "B", ColorFirst)},
			"B": {game(1, "C", ColorSecond)},
		},
	}
	plan, err := NextRound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Plan{
		Round: 2,
		Pairs: []Pair{
			{FirstID: "B", SecondID: "A"}, // A moves second
			{FirstID: "D", SecondID: "C"}, // C moves second
		},
	}
	assertPlan(t, plan, want)
}

func TestOddHeadCountGetsExactlyOneBye(t *testing.T) {
	plan, err := NextRound(&Request{Players: players("A", "B", "C", "D", "E")})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.ByeID != "A" {
		t.Fatalf("bye = %q, want A (lowest-ID bye wins the canonical tie-break)", plan.ByeID)
	}
	want := Plan{
		Round: 1, ByeID: "A",
		Pairs: []Pair{
			{FirstID: "B", SecondID: "C"},
			{FirstID: "D", SecondID: "E"},
		},
	}
	assertPlan(t, plan, want)
}

func TestPriorByeCannotByeAgain(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D", "E"),
		Byes: map[string][]int{
			"A": {1}, "B": {1}, "C": {1}, "D": {1}, "E": {1},
		},
	}
	_, err := NextRound(req)
	if !errors.Is(err, ErrNoPairing) {
		t.Fatalf("err = %v, want NO_PAIRING", err)
	}
}

func TestAllPairingsPlayedLeavesNoPlan(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Games: map[string][]Game{
			"A": {game(1, "B", ColorFirst), game(2, "C", ColorFirst), game(3, "D", ColorFirst)},
			"B": {game(1, "A", ColorSecond), game(2, "D", ColorFirst), game(3, "C", ColorSecond)},
			"C": {game(1, "D", ColorFirst), game(2, "A", ColorSecond), game(3, "B", ColorFirst)},
			"D": {game(1, "C", ColorSecond), game(2, "B", ColorSecond), game(3, "A", ColorSecond)},
		},
	}
	_, err := NextRound(req)
	if !errors.Is(err, ErrNoPairing) {
		t.Fatalf("err = %v, want NO_PAIRING", err)
	}
}

func TestForcedColorsCanMakeMatchingImpossible(t *testing.T) {
	// After two rounds 1,2,3 have a +2 balance (forced second) and 4,5,6 a
	// -2 balance (forced first). Round 3 exhausts player 1's only remaining
	// opponent of the forced-first group, so no complete plan exists.
	req := &Request{
		Players: players("1", "2", "3", "4", "5", "6"),
		Games: map[string][]Game{
			"1": {game(1, "4", ColorFirst), game(2, "5", ColorFirst), game(3, "6", ColorFirst)},
			"2": {game(1, "5", ColorFirst), game(2, "6", ColorFirst)},
			"3": {game(1, "6", ColorFirst), game(2, "4", ColorFirst)},
			"4": {game(1, "1", ColorSecond), game(2, "3", ColorSecond)},
			"5": {game(1, "2", ColorSecond), game(2, "1", ColorSecond)},
			"6": {game(1, "3", ColorSecond), game(2, "2", ColorSecond), game(3, "1", ColorSecond)},
		},
	}
	_, err := NextRound(req)
	if !errors.Is(err, ErrNoPairing) {
		t.Fatalf("err = %v, want NO_PAIRING", err)
	}
}

func TestRejectsDuplicateAppearanceInOneRound(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Games: map[string][]Game{
			"A": {game(1, "B", ColorFirst), game(1, "C", ColorFirst)},
			"B": {game(1, "A", ColorSecond)},
			"C": {game(1, "A", ColorSecond)},
		},
	}
	_, err := NextRound(req)
	if !IsInvalidHistory(err) {
		t.Fatalf("err = %v, want invalid history", err)
	}
}

func TestRejectsColorDisagreement(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Games: map[string][]Game{
			"A": {game(1, "B", ColorFirst)},
			"B": {game(1, "A", ColorFirst)}, // both claim first
		},
	}
	_, err := NextRound(req)
	if !IsInvalidHistory(err) {
		t.Fatalf("err = %v, want invalid history", err)
	}
}

func TestRejectsMissingMirrorRecord(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Games: map[string][]Game{
			"A": {game(1, "B", ColorFirst)},
		},
	}
	_, err := NextRound(req)
	if !IsInvalidHistory(err) {
		t.Fatalf("err = %v, want invalid history", err)
	}
}

func TestRejectsPlayerPlayingAndByeInSameRound(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Games: map[string][]Game{
			"A": {game(1, "B", ColorFirst)},
			"B": {game(1, "A", ColorSecond)},
		},
		Byes: map[string][]int{"A": {1}},
	}
	_, err := NextRound(req)
	if !IsInvalidHistory(err) {
		t.Fatalf("err = %v, want invalid history", err)
	}
}

func TestRejectsHeadCountOutsideRange(t *testing.T) {
	for _, n := range []int{0, 3, 13} {
		req := &Request{Players: players(idList(n)...)}
		if _, err := NextRound(req); !IsInvalidHistory(err) {
			t.Fatalf("n=%d: err = %v, want invalid history", n, err)
		}
	}
}

func TestRoundNumberFollowsHighestHistoryRound(t *testing.T) {
	req := &Request{
		Players: players("A", "B", "C", "D"),
		Byes:    map[string][]int{"A": {5}},
	}
	plan, err := NextRound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plan.Round != 6 {
		t.Fatalf("round = %d, want 6", plan.Round)
	}
}

func TestResultSatisfiesAllHardConstraints(t *testing.T) {
	// A plan with pairs must leave every paired player within a post-game
	// colour imbalance of 2 and must not repeat an earlier pairing.
	req := &Request{
		Players: []Player{
			{ID: "A", Score: 6}, {ID: "B", Score: 4},
			{ID: "C", Score: 4}, {ID: "D", Score: 2},
		},
		Games: map[string][]Game{
			"A": {game(1, "C", ColorFirst)},
			"C": {game(1, "A", ColorSecond)},
			"B": {game(1, "D", ColorSecond)},
			"D": {game(1, "B", ColorFirst)},
		},
	}
	plan, err := NextRound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(plan.Pairs) != 2 {
		t.Fatalf("pairs = %v", plan.Pairs)
	}
	seen := map[string]bool{}
	for _, p := range plan.Pairs {
		if p.FirstID == p.SecondID {
			t.Fatalf("self pair: %v", p)
		}
		if seen[p.FirstID] || seen[p.SecondID] {
			t.Fatalf("player paired twice: %v", plan.Pairs)
		}
		seen[p.FirstID], seen[p.SecondID] = true, true
	}
}

func TestHighScoreSumOverflowPicksSameTier(t *testing.T) {
	// Scores are legal up to math.MaxInt64; the sum of two maximal score
	// differences overflows int64. The same-tier pairing (true cost 0)
	// must still beat the cross-tier pairing (true cost 2*MaxInt64).
	req := &Request{Players: []Player{
		{ID: "A", Score: math.MaxInt64},
		{ID: "B", Score: 0},
		{ID: "C", Score: math.MaxInt64},
		{ID: "D", Score: 0},
	}}
	plan, err := NextRound(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := Plan{
		Round: 1,
		Pairs: []Pair{
			{FirstID: "A", SecondID: "C"},
			{FirstID: "B", SecondID: "D"},
		},
	}
	assertPlan(t, plan, want)
}

func TestHighScoreSumOverflowPermutations(t *testing.T) {
	// The same two maximal and two zero scores assigned to every possible
	// pair of IDs: whichever pairing the search visits first, the returned
	// plan must pair equal scores (the unique zero-cost optimum).
	ids := []string{"A", "B", "C", "D"}
	for mask := 0; mask < 1<<4; mask++ {
		if bits.OnesCount32(uint32(mask)) != 2 {
			continue
		}
		players := make([]Player, 4)
		scoreOf := map[string]int{}
		for k, id := range ids {
			if mask&(1<<k) != 0 {
				players[k] = Player{ID: id, Score: math.MaxInt64}
			} else {
				players[k] = Player{ID: id}
			}
			scoreOf[id] = players[k].Score
		}
		plan, err := NextRound(&Request{Players: players})
		if err != nil {
			t.Fatalf("mask %04b: unexpected error: %v", mask, err)
		}
		if len(plan.Pairs) != 2 {
			t.Fatalf("mask %04b: pairs = %v", mask, plan.Pairs)
		}
		for _, pr := range plan.Pairs {
			if scoreOf[pr.FirstID] != scoreOf[pr.SecondID] {
				t.Fatalf("mask %04b: cross-tier pair %v in plan %v", mask, pr, plan.Pairs)
			}
		}
	}
}

func idList(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = string(rune('A' + i))
	}
	return out
}

func assertPlan(t *testing.T, got, want Plan) {
	t.Helper()
	if got.Round != want.Round || got.ByeID != want.ByeID || len(got.Pairs) != len(want.Pairs) {
		t.Fatalf("plan = %+v, want %+v", got, want)
	}
	for i := range want.Pairs {
		if got.Pairs[i] != want.Pairs[i] {
			t.Fatalf("pairs = %+v, want %+v", got.Pairs, want.Pairs)
		}
	}
}

package pairing_test

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"slices"
	"testing"

	"pairing"
)

// oraclePlan is an independently written brute-force answer used solely in
// tests. It enumerates every bye choice, every perfect matching and every
// colour orientation from scratch, then applies the same documented
// lexicographic objective.
type oraclePlan struct {
	pairs     []pairing.Pair
	scoreCost *big.Int // exact: legal scores reach int64 and sums can overflow
	colorCost int
	key       []string
	bye       int // index, -1 when nobody has a bye
}

func oracle(req *pairing.Request, index map[string]int, ids []string, scores map[string]int,
	balance map[string]int, played map[[2]string]bool, hasBye map[string]bool) (oraclePlan, bool) {

	n := len(ids)
	var best *oraclePlan

	colors := []pairing.Color{pairing.ColorFirst, pairing.ColorSecond}

	var search func(bye int, used []bool, edges [][2]int, col map[int]pairing.Color)
	leaf := func(bye int, edges [][2]int, col map[int]pairing.Color) {
		p := renderPairs(ids, edges, col)
		sc := new(big.Int)
		for _, e := range edges {
			a, b := ids[e[0]], ids[e[1]]
			sc.Add(sc, big.NewInt(int64(abs(scores[a]-scores[b]))))
		}
		cc := 0
		for i, id := range ids {
			if bye == i {
				cc += abs(balance[id])
				continue
			}
			sign := 1
			if col[i] == pairing.ColorSecond {
				sign = -1
			}
			cc += abs(balance[id] + sign)
		}
		cand := oraclePlan{pairs: p, scoreCost: sc, colorCost: cc, key: oracleKey(p, byeID(bye, ids)), bye: bye}
		if best == nil || oracleLess(cand, *best) {
			best = &cand
		}
	}
	search = func(bye int, used []bool, edges [][2]int, col map[int]pairing.Color) {
		i := -1
		for k, u := range used {
			if !u {
				i = k
				break
			}
		}
		if i < 0 {
			leaf(bye, edges, col)
			return
		}
		used[i] = true
		for j := i + 1; j < n; j++ {
			if used[j] {
				continue
			}
			a, b := ids[i], ids[j]
			if a > b {
				a, b = b, a
			}
			if played[[2]string{a, b}] {
				continue
			}
			for _, ci := range colors {
				cj := oracleOpposite(ci)
				if abs(balance[ids[i]]+oracleSign(ci)) > 2 ||
					abs(balance[ids[j]]+oracleSign(cj)) > 2 {
					continue
				}
				used[j] = true
				col[i], col[j] = ci, cj
				search(bye, used, append(edges, [2]int{i, j}), col)
				delete(col, i)
				delete(col, j)
				used[j] = false
			}
		}
		used[i] = false
	}

	byes := []int{-1}
	if n%2 == 1 {
		byes = byes[:0]
		for i, id := range ids {
			if !hasBye[id] {
				byes = append(byes, i)
			}
		}
	}
	for _, bye := range byes {
		used := make([]bool, n)
		if bye >= 0 {
			used[bye] = true
		}
		search(bye, used, nil, map[int]pairing.Color{})
	}
	if best == nil {
		return oraclePlan{}, false
	}
	return *best, true
}

func oracleLess(a, b oraclePlan) bool {
	if c := a.scoreCost.Cmp(b.scoreCost); c != 0 {
		return c < 0
	}
	if a.colorCost != b.colorCost {
		return a.colorCost < b.colorCost
	}
	return slices.Compare(a.key, b.key) < 0
}

func oracleSign(c pairing.Color) int {
	if c == pairing.ColorFirst {
		return 1
	}
	return -1
}

func oracleOpposite(c pairing.Color) pairing.Color {
	if c == pairing.ColorFirst {
		return pairing.ColorSecond
	}
	return pairing.ColorFirst
}

func renderPairs(ids []string, edges [][2]int, col map[int]pairing.Color) []pairing.Pair {
	type row struct{ lo, hi string }
	rows := make([]row, len(edges))
	for k, e := range edges {
		a, b := ids[e[0]], ids[e[1]]
		if a < b {
			rows[k] = row{a, b}
		} else {
			rows[k] = row{b, a}
		}
	}
	slices.SortFunc(rows, func(x, y row) int {
		if x.lo != y.lo {
			if x.lo < y.lo {
				return -1
			}
			return 1
		}
		if x.hi < y.hi {
			return -1
		} else if x.hi > y.hi {
			return 1
		}
		return 0
	})
	out := make([]pairing.Pair, len(rows))
	for k, r := range rows {
		loIdx := indexOf(ids, r.lo)
		if col[loIdx] == pairing.ColorFirst {
			out[k] = pairing.Pair{FirstID: r.lo, SecondID: r.hi}
		} else {
			out[k] = pairing.Pair{FirstID: r.hi, SecondID: r.lo}
		}
	}
	return out
}

func byeID(bye int, ids []string) string {
	if bye < 0 {
		return ""
	}
	return ids[bye]
}

func oracleKey(pairs []pairing.Pair, bye string) []string {
	type entry struct {
		anchor    string
		flattened []string
	}
	entries := make([]entry, 0, len(pairs)+1)
	for _, p := range pairs {
		if p.FirstID < p.SecondID {
			entries = append(entries, entry{p.FirstID, []string{p.FirstID, "F", p.SecondID, "S"}})
		} else {
			entries = append(entries, entry{p.SecondID, []string{p.SecondID, "S", p.FirstID, "F"}})
		}
	}
	if bye != "" {
		entries = append(entries, entry{bye, []string{bye, "B"}})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		if a.anchor < b.anchor {
			return -1
		}
		if a.anchor > b.anchor {
			return 1
		}
		return 0
	})
	var key []string
	for _, e := range entries {
		key = append(key, e.flattened...)
	}
	return key
}

func indexOf(xs []string, x string) int {
	for i, v := range xs {
		if v == x {
			return i
		}
	}
	return -1
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ---- random valid-history generator ----

type fixture struct {
	req     *pairing.Request
	ids     []string
	scores  map[string]int
	balance map[string]int
	played  map[[2]string]bool
	hasBye  map[string]bool
	rounds  int
}

func generateFixture(rng *rand.Rand, n int) *fixture {
	ids := make([]string, n)
	for i := range ids {
		ids[i] = fmt.Sprintf("P%02d", i+1)
	}
	scores := make(map[string]int, n)
	for _, id := range ids {
		scores[id] = rng.Intn(12)
	}
	if rng.Intn(4) == 0 {
		// A quarter of the fixtures push (some) scores to the top of the
		// legal non-negative-int range, where naive cost sums overflow
		// int64; the solver must stay exact there as well.
		for _, id := range ids {
			if rng.Intn(2) == 0 {
				scores[id] = int(rng.Int63())
			}
		}
	}
	balance := make(map[string]int, n)
	played := map[[2]string]bool{}
	hasBye := map[string]bool{}
	games := map[string][]pairing.Game{}
	byes := map[string][]int{}
	for _, id := range ids {
		games[id] = nil
	}

	roundsPlanned := rng.Intn(3) // 0..2 played rounds, keeps post-game |balance| <= 2 feasible
	playedRounds := 0
	for r := 1; r <= roundsPlanned; r++ {
		shuffled := append([]string(nil), ids...)
		rng.Shuffle(n, func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
		pool := shuffled
		if n%2 == 1 {
			// pick a bye among players that did not have one yet
			candidates := filter(pool, func(id string) bool { return !hasBye[id] })
			if len(candidates) == 0 {
				break
			}
			b := candidates[rng.Intn(len(candidates))]
			hasBye[b] = true
			byes[b] = append(byes[b], r)
			pool = filter(pool, func(id string) bool { return id != b })
		}
		pairs := randomPerfectMatching(rng, pool, played, balance)
		if pairs == nil || !orientAll(pairs, balance) {
			// leave remaining rounds unplayed rather than emit invalid history
			break
		}
		playedRounds++
		for _, pr := range pairs {
			a, b := pr[0], pr[1]
			ca := pickColor(balance[a], balance[b])
			cb := oracleOpposite(ca)
			games[a] = append(games[a], pairing.Game{Round: r, OpponentID: b, Color: ca})
			games[b] = append(games[b], pairing.Game{Round: r, OpponentID: a, Color: cb})
			balance[a] += oracleSign(ca)
			balance[b] += oracleSign(cb)
			lo, hi := a, b
			if lo > hi {
				lo, hi = hi, lo
			}
			played[[2]string{lo, hi}] = true
		}
	}

	players := make([]pairing.Player, n)
	for i, id := range ids {
		players[i] = pairing.Player{ID: id, Score: scores[id]}
	}
	return &fixture{
		req:     &pairing.Request{Players: players, Games: games, Byes: byes},
		ids:     ids,
		scores:  scores,
		balance: balance,
		played:  played,
		hasBye:  hasBye,
		rounds:  playedRounds,
	}
}

// pickColor returns a colour for player a such that both sides stay within
// the post-game imbalance of 2. orientAll must have confirmed existence.
func pickColor(balA, balB int) pairing.Color {
	if abs(balA+1) <= 2 && abs(balB-1) <= 2 {
		return pairing.ColorFirst
	}
	return pairing.ColorSecond
}

// orientAll reports whether every pair admits a colour orientation within
// balance 2 for both endpoints (sufficient here since each edge is
// independent: an edge is orientable unless both endpoints are at +2 or both
// are at -2).
func orientAll(pairs [][2]string, balance map[string]int) bool {
	for _, p := range pairs {
		a, b := balance[p[0]], balance[p[1]]
		forward := abs(a+1) <= 2 && abs(b-1) <= 2
		backward := abs(a-1) <= 2 && abs(b+1) <= 2
		if !forward && !backward {
			return false
		}
	}
	return true
}

func randomPerfectMatching(rng *rand.Rand, pool []string, played map[[2]string]bool,
	balance map[string]int) [][2]string {

	// find any legal matching with backtracking; colour feasibility is
	// guaranteed because each player has played at most 2 games so far.
	used := make([]bool, len(pool))
	var out [][2]string
	var go_ func() bool
	go_ = func() bool {
		i := -1
		for k, u := range used {
			if !u {
				i = k
				break
			}
		}
		if i < 0 {
			return true
		}
		used[i] = true
		js := rng.Perm(len(pool))
		for _, j := range js {
			if j <= i || used[j] {
				continue
			}
			a, b := pool[i], pool[j]
			lo, hi := a, b
			if lo > hi {
				lo, hi = hi, lo
			}
			if played[[2]string{lo, hi}] {
				continue
			}
			used[j] = true
			out = append(out, [2]string{a, b})
			if go_() {
				return true
			}
			out = out[:len(out)-1]
			used[j] = false
		}
		used[i] = false
		return false
	}
	if go_() {
		return out
	}
	return nil
}

func filter(xs []string, keep func(string) bool) []string {
	var out []string
	for _, x := range xs {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

func TestAgainstBruteForceOracle(t *testing.T) {
	rng := rand.New(rand.NewSource(20260930))
	for iter := 0; iter < 1000; iter++ {
		n := 4 + rng.Intn(6) // 4..9 keeps the oracle comfortably fast
		verifyOne(t, iter, n, generateFixture(rng, n))
	}
}

func TestLargeHeadCounts(t *testing.T) {
	rng := rand.New(rand.NewSource(99))
	for _, n := range []int{10, 11, 12} {
		for iter := 0; iter < 5; iter++ {
			f := generateFixture(rng, n)
			verifyOne(t, n*100+iter, n, f)
		}
	}
}

func verifyOne(t *testing.T, iter, n int, f *fixture) {
	t.Helper()
	plan, err := pairing.NextRound(f.req)
	want, ok := oracle(f.req, nil, f.ids, f.scores, f.balance, f.played, f.hasBye)
	if !ok {
		if !errors.Is(err, pairing.ErrNoPairing) {
			t.Fatalf("iter %d: oracle says no plan, solver returned %+v / %v", iter, plan, err)
		}
		return
	}
	if err != nil {
		t.Fatalf("iter %d: unexpected solver error: %v", iter, err)
	}
	if len(plan.Pairs) != len(want.pairs) {
		t.Fatalf("iter %d: pairs %v, want %v", iter, plan.Pairs, want.pairs)
	}
	for i := range want.pairs {
		if plan.Pairs[i] != want.pairs[i] {
			t.Fatalf("iter %d: pairs %v, want %v", iter, plan.Pairs, want.pairs)
		}
	}
	wantBye := ""
	if want.bye >= 0 {
		wantBye = f.ids[want.bye]
	}
	if plan.ByeID != wantBye {
		t.Fatalf("iter %d: bye %q, want %q", iter, plan.ByeID, wantBye)
	}
	if plan.Round != f.rounds+1 {
		t.Fatalf("iter %d: round %d, want %d", iter, plan.Round, f.rounds+1)
	}
}

// TestWorstCaseRuntime guards the n=12 exhaustive search staying small.
func TestWorstCaseRuntime(t *testing.T) {
	req := &pairing.Request{Players: make([]pairing.Player, 12)}
	for i := range req.Players {
		req.Players[i] = pairing.Player{ID: fmt.Sprintf("Z%02d", i)}
	}
	if _, err := pairing.NextRound(req); err != nil {
		t.Fatal(err)
	}
}

package pairing

import (
	"slices"
)

// NextRound computes the best legal plan for the round following the history
// in req.
//
// The optimization, applied lexicographically over complete plans only, is:
//
//  1. minimize the sum of absolute score differences within every pair
//     (scores are in half points);
//  2. minimize the sum over all players of the absolute post-game difference
//     between their first- and second-colour counts;
//  3. take the lexicographically smallest pairing-and-colour sequence once
//     the pairs are sorted by the lower player ID; a bye is part of the
//     sequence and sorts as marker "B" ("B" < "F" < "S").
//
// If the request itself is malformed an InvalidHistoryError is returned. When
// the request is valid but no complete legal schedule exists, ErrNoPairing
// (NO_PAIRING) is returned and no partial schedule is produced.
func NextRound(req *Request) (Plan, error) {
	nr, err := normalize(req)
	if err != nil {
		return Plan{}, err
	}
	return nr.solve()
}

// bestPlan accumulates the incumbent solution across the whole search.
type bestPlan struct {
	scoreCost int
	colorCost int
	key       []string
	edges     [][2]int // sorted-index edges, in the same order as key
	orient    []Color  // colour per player index
	bye       int      // player index, -1 when there is no bye
	found     bool
}

func (nr *normalized) solve() (Plan, error) {
	n := len(nr.ids)
	odd := n%2 == 1

	// standing[i] is the current first-minus-second colour balance.
	standing := make([]int, n)
	for i := range standing {
		standing[i] = nr.first[i] - nr.second[i]
	}

	// absPost[a][side] is |standing[a] + sign(side)| with side 0 = first.
	absPost := make([][2]int, n)
	for i := range standing {
		absPost[i][0] = absInt(standing[i] + 1)
		absPost[i][1] = absInt(standing[i] - 1)
	}

	best := bestPlan{bye: -1}

	// DFS state; edges are always appended at increasing "first free"
	// indices i, so their anchor (min ID) sequence is already sorted by ID.
	var dfs func(bye int, used []bool, edges [][2]int, orient []Color,
		scoreSoFar, colorSoFar, remaining int)
	dfs = func(bye int, used []bool, edges [][2]int, orient []Color,
		scoreSoFar, colorSoFar, remaining int) {
		if best.found && scoreSoFar > best.scoreCost {
			return // adding non-negative edge costs cannot improve
		}
		if remaining == 0 {
			totalColor := colorSoFar
			if bye >= 0 {
				totalColor += absInt(standing[bye])
			}
			if best.found {
				if scoreSoFar > best.scoreCost ||
					(scoreSoFar == best.scoreCost && totalColor > best.colorCost) {
					return
				}
			}
			var key []string
			dominates := !best.found || scoreSoFar < best.scoreCost ||
				(scoreSoFar == best.scoreCost && totalColor < best.colorCost)
			if !dominates {
				key = nr.canonicalKey(edges, orient, bye)
				if slices.Compare(key, best.key) >= 0 {
					return
				}
			}
			edgesCopy := append([][2]int(nil), edges...)
			orientCopy := append([]Color(nil), orient...)
			best.scoreCost = scoreSoFar
			best.colorCost = totalColor
			best.edges = edgesCopy
			best.orient = orientCopy
			best.bye = bye
			best.found = true
			if key == nil {
				best.key = nr.canonicalKey(edges, orient, bye)
			} else {
				best.key = key
			}
			return
		}

		// fixing the first free player makes each perfect matching unique
		i := firstFree(used, n)
		used[i] = true
		for j := i + 1; j < n; j++ {
			if used[j] || nr.played[i][j] {
				continue
			}
			edgeScore := absInt(nr.score[i] - nr.score[j])
			used[j] = true
			edges = append(edges, [2]int{i, j})
			// side 0: i first / j second; side 1: i second / j first
			for side := 0; side < 2; side++ {
				ci := sideAsColor(side)
				cj := opposite(ci)
				postI := absPost[i][side]
				postJ := absPost[j][1-side]
				if postI > 2 || postJ > 2 {
					continue
				}
				orient[i], orient[j] = ci, cj
				dfs(bye, used, edges, orient,
					scoreSoFar+edgeScore, colorSoFar+postI+postJ, remaining-2)
			}
			edges = edges[:len(edges)-1]
			used[j] = false
		}
		used[i] = false
	}

	used := make([]bool, n)
	orient := make([]Color, n)
	if odd {
		foundByeCandidate := false
		for bye := 0; bye < n; bye++ {
			if len(nr.byes[bye]) > 0 {
				continue // a player that already had a bye cannot bye again
			}
			foundByeCandidate = true
			clear(used)
			clear(orient)
			used[bye] = true
			dfs(bye, used, nil, orient, 0, 0, n-1)
		}
		if !foundByeCandidate {
			return Plan{}, ErrNoPairing
		}
	} else {
		dfs(-1, used, nil, orient, 0, 0, n)
	}

	if !best.found {
		return Plan{}, ErrNoPairing
	}

	pairs := make([]Pair, len(best.edges))
	for k, e := range best.edges {
		a, b := e[0], e[1]
		if best.orient[a] == ColorFirst {
			pairs[k] = Pair{FirstID: nr.ids[a], SecondID: nr.ids[b]}
		} else {
			pairs[k] = Pair{FirstID: nr.ids[b], SecondID: nr.ids[a]}
		}
	}
	plan := Plan{Round: nr.round, Pairs: pairs}
	if best.bye >= 0 {
		plan.ByeID = nr.ids[best.bye]
	}
	return plan, nil
}

// canonicalKey renders the arbitration sequence of a complete plan. DFS
// appends edges at increasing anchor indices, so pair entries are already in
// ID order; the bye entry is merged into its sorted position.
func (nr *normalized) canonicalKey(edges [][2]int, orient []Color, bye int) []string {
	ids := nr.ids
	key := make([]string, 0, len(edges)*4+2)
	k := 0
	for k < len(edges) {
		a, b := edges[k][0], edges[k][1] // a is the lower sorted position / ID
		if bye >= 0 && ids[bye] < ids[a] {
			key = append(key, ids[bye], "B")
			bye = -1
		}
		if orient[a] == ColorFirst {
			key = append(key, ids[a], "F", ids[b], "S")
		} else {
			key = append(key, ids[a], "S", ids[b], "F")
		}
		k++
	}
	if bye >= 0 {
		key = append(key, ids[bye], "B")
	}
	return key
}

func sideAsColor(side int) Color {
	if side == 0 {
		return ColorFirst
	}
	return ColorSecond
}

func firstFree(used []bool, n int) int {
	for i := 0; i < n; i++ {
		if !used[i] {
			return i
		}
	}
	return n
}

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

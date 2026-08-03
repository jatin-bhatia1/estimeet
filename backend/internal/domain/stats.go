package domain

import (
	"math"
	"sort"
)

// DistributionEntry counts how many players picked one card.
type DistributionEntry struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// Stats summarises a revealed round.
type Stats struct {
	VoteCount    int                 `json:"voteCount"`
	Consensus    bool                `json:"consensus"`
	Min          *string             `json:"min,omitempty"`
	Max          *string             `json:"max,omitempty"`
	Average      *float64            `json:"average,omitempty"`
	Median       *float64            `json:"median,omitempty"`
	Suggested    *string             `json:"suggested,omitempty"`
	Spread       int                 `json:"spread"`
	Distribution []DistributionEntry `json:"distribution"`
}

// Summarize computes the round statistics. Non-numeric cards ("?" and coffee)
// are counted in the distribution but excluded from min/max/average/median.
func Summarize(votes []Vote, deck []string) Stats {
	st := Stats{VoteCount: len(votes), Distribution: []DistributionEntry{}}
	if len(votes) == 0 {
		return st
	}

	counts := make(map[string]int, len(votes))
	numeric := make([]float64, 0, len(votes))
	numericCards := make([]string, 0, len(votes))
	for _, v := range votes {
		counts[v.Value]++
		if n, ok := NumericValue(v.Value); ok {
			numeric = append(numeric, n)
			numericCards = append(numericCards, v.Value)
		}
	}

	// Distribution ordered by deck order so the UI renders a stable histogram.
	seen := make(map[string]bool, len(counts))
	for _, card := range deck {
		if c, ok := counts[card]; ok {
			st.Distribution = append(st.Distribution, DistributionEntry{Value: card, Count: c})
			seen[card] = true
		}
	}
	extras := make([]string, 0)
	for card := range counts {
		if !seen[card] {
			extras = append(extras, card)
		}
	}
	sort.Strings(extras)
	for _, card := range extras {
		st.Distribution = append(st.Distribution, DistributionEntry{Value: card, Count: counts[card]})
	}

	st.Consensus = len(counts) == 1

	if len(numeric) == 0 {
		return st
	}

	idx := make([]int, len(numeric))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return numeric[idx[a]] < numeric[idx[b]] })

	minCard := numericCards[idx[0]]
	maxCard := numericCards[idx[len(idx)-1]]
	st.Min = &minCard
	st.Max = &maxCard

	sorted := make([]float64, len(numeric))
	for i, j := range idx {
		sorted[i] = numeric[j]
	}

	var sum float64
	for _, n := range sorted {
		sum += n
	}
	avg := round2(sum / float64(len(sorted)))
	st.Average = &avg

	median := medianOf(sorted)
	st.Median = &median

	if s, ok := nearestCard(deck, avg); ok {
		st.Suggested = &s
	}

	// Spread = how many deck steps separate the lowest and highest numeric card.
	st.Spread = deckDistance(deck, minCard, maxCard)
	return st
}

func medianOf(sorted []float64) float64 {
	n := len(sorted)
	if n%2 == 1 {
		return round2(sorted[n/2])
	}
	return round2((sorted[n/2-1] + sorted[n/2]) / 2)
}

// nearestCard finds the numeric deck card closest to value, rounding up on ties
// so the team never under-commits.
func nearestCard(deck []string, value float64) (string, bool) {
	best := ""
	bestDist := math.MaxFloat64
	for _, card := range deck {
		n, ok := NumericValue(card)
		if !ok {
			continue
		}
		d := math.Abs(n - value)
		if d < bestDist || (d == bestDist && n > mustNumeric(best)) {
			best, bestDist = card, d
		}
	}
	return best, best != ""
}

func mustNumeric(card string) float64 {
	n, _ := NumericValue(card)
	return n
}

func deckDistance(deck []string, a, b string) int {
	ia, ib := -1, -1
	for i, card := range deck {
		if card == a && ia == -1 {
			ia = i
		}
		if card == b {
			ib = i
		}
	}
	if ia == -1 || ib == -1 {
		return 0
	}
	if ib < ia {
		ia, ib = ib, ia
	}
	return ib - ia
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

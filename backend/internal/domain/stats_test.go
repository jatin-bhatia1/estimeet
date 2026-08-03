package domain_test

import (
	"testing"

	"github.com/jatin-bhatia1/estimeet/backend/internal/domain"
)

func votes(values ...string) []domain.Vote {
	out := make([]domain.Vote, 0, len(values))
	for i, v := range values {
		out = append(out, domain.Vote{TopicID: "t", ParticipantID: string(rune('a' + i)), Value: v})
	}
	return out
}

func TestSummarizeConsensus(t *testing.T) {
	st := domain.Summarize(votes("5", "5", "5"), domain.FibonacciDeck)

	if !st.Consensus {
		t.Fatalf("expected consensus")
	}
	if st.Average == nil || *st.Average != 5 {
		t.Fatalf("average = %v, want 5", st.Average)
	}
	if st.Suggested == nil || *st.Suggested != "5" {
		t.Fatalf("suggested = %v, want 5", st.Suggested)
	}
	if st.Spread != 0 {
		t.Fatalf("spread = %d, want 0", st.Spread)
	}
}

func TestSummarizeIgnoresSpecialCardsInMath(t *testing.T) {
	st := domain.Summarize(votes("3", "8", domain.CardUnknown, domain.CardCoffee), domain.FibonacciDeck)

	if st.VoteCount != 4 {
		t.Fatalf("voteCount = %d, want 4", st.VoteCount)
	}
	if st.Average == nil || *st.Average != 5.5 {
		t.Fatalf("average = %v, want 5.5", st.Average)
	}
	if st.Min == nil || *st.Min != "3" || st.Max == nil || *st.Max != "8" {
		t.Fatalf("min/max = %v/%v, want 3/8", st.Min, st.Max)
	}
	if st.Consensus {
		t.Fatalf("expected no consensus")
	}
	if len(st.Distribution) != 4 {
		t.Fatalf("distribution entries = %d, want 4", len(st.Distribution))
	}
}

func TestSummarizeSuggestsNearestCardRoundingUp(t *testing.T) {
	// Average of 5 and 13 is 9, which sits exactly between 8 and 13.
	st := domain.Summarize(votes("5", "13"), domain.FibonacciDeck)

	if st.Suggested == nil || *st.Suggested != "8" {
		t.Fatalf("suggested = %v, want 8 (closest to 9)", st.Suggested)
	}
	if st.Median == nil || *st.Median != 9 {
		t.Fatalf("median = %v, want 9", st.Median)
	}
	if st.Spread != 2 {
		t.Fatalf("spread = %d, want 2 (5 -> 8 -> 13)", st.Spread)
	}
}

func TestSummarizeAllSpecialCards(t *testing.T) {
	st := domain.Summarize(votes(domain.CardUnknown, domain.CardUnknown), domain.FibonacciDeck)

	if !st.Consensus {
		t.Fatalf("expected consensus on identical cards")
	}
	if st.Average != nil || st.Median != nil || st.Suggested != nil {
		t.Fatalf("expected no numeric statistics, got avg=%v median=%v suggested=%v",
			st.Average, st.Median, st.Suggested)
	}
}

func TestNumericValue(t *testing.T) {
	if _, ok := domain.NumericValue(domain.CardUnknown); ok {
		t.Fatalf("%q must not be numeric", domain.CardUnknown)
	}
	if v, ok := domain.NumericValue("21"); !ok || v != 21 {
		t.Fatalf("NumericValue(21) = %v, %v", v, ok)
	}
}

func TestNewRoomCodeIsUnambiguous(t *testing.T) {
	for range 50 {
		code, err := domain.NewRoomCode()
		if err != nil {
			t.Fatal(err)
		}
		if len(code) != 6 {
			t.Fatalf("code %q must be 6 characters", code)
		}
		for _, r := range code {
			if r == '0' || r == 'O' || r == '1' || r == 'I' {
				t.Fatalf("code %q contains an ambiguous character", code)
			}
		}
	}
}

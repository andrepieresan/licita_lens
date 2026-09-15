package domain

import (
	"testing"
	"time"
)

func TestRankIsDeterministicAndExplainable(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	profile := CommercialProfile{ID: "p1", States: []string{"PR"}, RequiredTerms: []string{"notebook"}, MaximumValueCents: 2_000_000}
	opportunity := Opportunity{ID: "o1", Object: "Aquisição de notebook corporativo", State: "PR", EstimatedValueCents: 1_000_000, PublishedAt: now.Add(-24 * time.Hour)}
	match, ok := Rank(RankInput{Profile: profile, Opportunity: opportunity, SemanticSimilarity: .9, CompetitionScore: .7, Now: now})
	if !ok {
		t.Fatal("expected opportunity to match")
	}
	if match.Score < 70 || len(match.Evidence) != 4 {
		t.Fatalf("unexpected match: %#v", match)
	}
}

func TestRankRejectsExcludedTerm(t *testing.T) {
	profile := CommercialProfile{ExcludedTerms: []string{"usado"}}
	opportunity := Opportunity{Object: "Notebook usado"}
	if _, ok := Rank(RankInput{Profile: profile, Opportunity: opportunity, Now: time.Now()}); ok {
		t.Fatal("expected rejection")
	}
}

func BenchmarkRank(b *testing.B) {
	now := time.Now().UTC()
	input := RankInput{Profile: CommercialProfile{ID: "p1", Description: "notebooks corporativos", States: []string{"PR"}}, Opportunity: Opportunity{ID: "o1", Object: "Aquisição de notebooks corporativos", State: "PR", PublishedAt: now}, SemanticSimilarity: .91, CompetitionScore: .62, Now: now}
	b.ReportAllocs()
	for range b.N {
		_, _ = Rank(input)
	}
}

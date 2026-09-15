package notifications

import (
	"strings"
	"time"
	"unicode"

	"licitalens.dev/backend/internal/domain"
)

func MatchOpportunity(profile domain.CommercialProfile, opportunity domain.Opportunity) (domain.Match, bool) {
	similarity := lexicalSimilarity(profile.Description+" "+strings.Join(profile.Keywords, " "), opportunity.Object)
	return domain.Rank(domain.RankInput{
		Profile:            profile,
		Opportunity:        opportunity,
		SemanticSimilarity: similarity,
		CompetitionScore:   0.5,
		Now:                time.Now().UTC(),
	})
}

func lexicalSimilarity(a, b string) float64 {
	left := tokenSet(a)
	right := tokenSet(b)
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	overlap := 0
	for token := range left {
		if right[token] {
			overlap++
		}
	}
	return float64(overlap) / float64(len(left))
}

func tokenSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, token := range strings.Fields(strings.ToLower(stripPunct(value))) {
		if len(token) < 3 {
			continue
		}
		result[token] = true
	}
	return result
}

func stripPunct(value string) string {
	var b strings.Builder
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return b.String()
}

package domain

import (
	"math"
	"strings"
	"time"
)

type RankInput struct {
	Profile            CommercialProfile
	Opportunity        Opportunity
	SemanticSimilarity float64
	CompetitionScore   float64
	Now                time.Time
}

func Rank(input RankInput) (Match, bool) {
	p, o := input.Profile, input.Opportunity
	text := strings.ToLower(o.Object)
	if !containsAll(text, p.RequiredTerms) || containsAny(text, p.ExcludedTerms) {
		return Match{}, false
	}
	if len(p.States) > 0 && !containsFold(p.States, o.State) {
		return Match{}, false
	}
	if len(p.Modalities) > 0 && !containsInt(p.Modalities, o.ModalityCode) {
		return Match{}, false
	}
	if p.MinimumValueCents > 0 && o.EstimatedValueCents < p.MinimumValueCents {
		return Match{}, false
	}
	if p.MaximumValueCents > 0 && o.EstimatedValueCents > p.MaximumValueCents {
		return Match{}, false
	}

	semantic := clamp(input.SemanticSimilarity)
	competition := clamp(input.CompetitionScore)
	age := input.Now.Sub(o.PublishedAt).Hours()
	recency := clamp(1 - age/(24*30))
	financial := financialFit(p, o)
	total := semantic*0.55 + recency*0.20 + financial*0.15 + competition*0.10

	evidence := []string{
		"similaridade semântica calculada sobre objeto e itens",
		"recência calculada a partir da publicação oficial",
		"adequação à faixa financeira configurada",
		"concorrência baseada no histórico disponível",
	}
	return Match{
		ProfileID: p.ID, OpportunityID: o.ID,
		Score:     int(math.Round(total * 100)),
		Breakdown: ScoreBreakdown{Semantic: semantic, Recency: recency, Financial: financial, Competition: competition},
		Evidence:  evidence, CalculatedAt: input.Now.UTC(),
	}, true
}

func financialFit(p CommercialProfile, o Opportunity) float64 {
	if p.MinimumValueCents == 0 && p.MaximumValueCents == 0 {
		return .5
	}
	if p.MaximumValueCents <= p.MinimumValueCents {
		return 1
	}
	midpoint := float64(p.MinimumValueCents+p.MaximumValueCents) / 2
	halfRange := float64(p.MaximumValueCents-p.MinimumValueCents) / 2
	return clamp(1 - math.Abs(float64(o.EstimatedValueCents)-midpoint)/halfRange)
}

func containsAll(text string, terms []string) bool {
	for _, term := range terms {
		if !strings.Contains(text, strings.ToLower(strings.TrimSpace(term))) {
			return false
		}
	}
	return true
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if term != "" && strings.Contains(text, strings.ToLower(strings.TrimSpace(term))) {
			return true
		}
	}
	return false
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func containsInt(values []int, wanted int) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func clamp(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

package domain

import "time"

type Role string

const (
	RoleOwner   Role = "owner"
	RoleAdmin   Role = "admin"
	RoleAnalyst Role = "analyst"
	RoleViewer  Role = "viewer"
)

type CommercialProfile struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organization_id"`
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Keywords          []string  `json:"keywords"`
	Categories        []string  `json:"categories"`
	States            []string  `json:"states"`
	Municipalities    []string  `json:"municipalities"`
	Modalities        []int     `json:"modalities"`
	RequiredTerms     []string  `json:"required_terms"`
	ExcludedTerms     []string  `json:"excluded_terms"`
	MinimumValueCents int64     `json:"minimum_value_cents"`
	MaximumValueCents int64     `json:"maximum_value_cents"`
	CreatedAt         time.Time `json:"created_at"`
}

type Opportunity struct {
	ID                  string    `json:"id"`
	Source              string    `json:"source"`
	SourceID            string    `json:"source_id"`
	Object              string    `json:"object"`
	OrganizationName    string    `json:"organization_name"`
	State               string    `json:"state"`
	Municipality        string    `json:"municipality"`
	ModalityCode        int       `json:"modality_code"`
	EstimatedValueCents int64     `json:"estimated_value_cents"`
	PublishedAt         time.Time `json:"published_at"`
	ProposalDeadline    time.Time `json:"proposal_deadline"`
	UpdatedAt           time.Time `json:"updated_at"`
	SourceURL           string    `json:"source_url"`
}

type ScoreBreakdown struct {
	Semantic    float64 `json:"semantic"`
	Recency     float64 `json:"recency"`
	Financial   float64 `json:"financial"`
	Competition float64 `json:"competition"`
}

type Match struct {
	ProfileID     string         `json:"profile_id"`
	OpportunityID string         `json:"opportunity_id"`
	Score         int            `json:"score"`
	Breakdown     ScoreBreakdown `json:"breakdown"`
	Evidence      []string       `json:"evidence"`
	Explanation   string         `json:"explanation,omitempty"`
	PromptVersion string         `json:"prompt_version,omitempty"`
	CalculatedAt  time.Time      `json:"calculated_at"`
}

type PriceBenchmark struct {
	Category      string `json:"category"`
	State         string `json:"state"`
	SampleSize    int64  `json:"sample_size"`
	MedianCents   int64  `json:"median_cents"`
	Percentile25  int64  `json:"percentile_25_cents"`
	Percentile75  int64  `json:"percentile_75_cents"`
	DeviationNote string `json:"deviation_note,omitempty"`
}

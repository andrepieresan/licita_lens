package domain

import "time"

type DealStage string

const (
	StageProspecting DealStage = "prospecting"
	StageAnalysis    DealStage = "analysis"
	StageProposal    DealStage = "proposal"
	StageNegotiation DealStage = "negotiation"
	StageWon         DealStage = "won"
	StageLost        DealStage = "lost"
)

func (s DealStage) Valid() bool {
	switch s {
	case StageProspecting, StageAnalysis, StageProposal, StageNegotiation, StageWon, StageLost:
		return true
	default:
		return false
	}
}

type Deal struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organization_id"`
	OpportunityID        string     `json:"opportunity_id,omitempty"`
	Title                string     `json:"title"`
	BuyerName            string     `json:"buyer_name"`
	Stage                DealStage  `json:"stage"`
	EstimatedValueCents  int64      `json:"estimated_value_cents"`
	NextFollowUpAt       *time.Time `json:"next_follow_up_at,omitempty"`
	ClosedAt             *time.Time `json:"closed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	LastFollowUpNote     string     `json:"last_follow_up_note,omitempty"`
}

type DealFollowUp struct {
	ID          string     `json:"id"`
	DealID      string     `json:"deal_id"`
	Note        string     `json:"note"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Account struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	SubjectID string    `json:"subject_id"`
	CreatedAt time.Time `json:"created_at"`
}

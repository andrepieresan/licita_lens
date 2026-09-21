package notifications

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"licitalens.dev/backend/internal/billing"
	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/store"
)

const minAlertScore = 55

type WorkerConfig struct {
	SMTP                SMTPConfig
	DemoMode            bool
	CloudMode           bool
	MinMatchScore       int
	ExpoAccessToken     string
	OpportunityLookback time.Duration
	UseLogExpoPush      bool
}

type Worker struct {
	store    store.DataStore
	dispatch *Dispatcher
	log      *slog.Logger
	cfg      WorkerConfig
}

func NewWorker(data store.DataStore, logger *slog.Logger, cfg WorkerConfig) *Worker {
	if cfg.MinMatchScore <= 0 {
		cfg.MinMatchScore = minAlertScore
	}
	expoToken := cfg.ExpoAccessToken
	if expoToken == "" {
		expoToken = os.Getenv("EXPO_ACCESS_TOKEN")
	}
	useLogExpo := cfg.UseLogExpoPush || os.Getenv("EXPO_PUSH_LOG_ONLY") == "true"
	var expoProvider Provider = NewExpoPushProvider(expoToken)
	if useLogExpo {
		expoProvider = NewLogProvider(ExpoPush)
	}
	providers := []Provider{expoProvider}
	if cfg.SMTP.Host != "" {
		providers = append(providers, NewSMTP(cfg.SMTP))
	} else if cfg.DemoMode {
		providers = append(providers, NewLogProvider(Email))
	}
	return &Worker{
		store:    data,
		dispatch: New(providers...),
		log:      logger,
		cfg:      cfg,
	}
}

func (w *Worker) Run(ctx context.Context) (domain.NotificationRunResult, error) {
	result := domain.NotificationRunResult{}
	targets, err := w.store.NotificationTargets(ctx)
	if err != nil {
		return result, err
	}
	result.Organizations = len(targets)
	now := time.Now().UTC()
	opportunities, err := w.store.Opportunities(ctx)
	if err != nil {
		return result, err
	}

	for _, target := range targets {
		if err := w.processTarget(ctx, target, opportunities, now, &result); err != nil {
			w.log.Warn("notification target failed", "organization_id", target.OrganizationID, "error", err)
			result.Errors++
		}
	}
	return result, nil
}

func (w *Worker) processTarget(ctx context.Context, target domain.NotificationTarget, opportunities []domain.Opportunity, now time.Time, result *domain.NotificationRunResult) error {
	profiles, err := w.store.Profiles(ctx, target.OrganizationID)
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return nil
	}
	cursor, err := w.store.NotificationOpportunityCursor(ctx, target.OrganizationID)
	if err != nil {
		return err
	}
	cutoff := opportunityCutoff(cursor, now, w.cfg.OpportunityLookback)

	emailChannel := Channel{
		Type:           Email,
		OrganizationID: target.OrganizationID,
		Destination:    target.OwnerEmail,
		Enabled:        target.Preferences.Email,
		ConsentedAt:    now,
	}

	for _, opportunity := range opportunities {
		if !w.shouldAlertOpportunity(profiles, opportunity) {
			continue
		}
		if !isNewOpportunity(opportunity, cutoff) {
			result.SkippedStale++
			continue
		}

		if target.Preferences.Email && target.OwnerEmail != "" {
			sent, err := w.store.NotificationAlertSent(ctx, target.OrganizationID, "opportunity", opportunity.ID)
			if err != nil {
				return err
			}
			if sent {
				result.SkippedDedup++
			} else if w.cfg.DemoMode || !w.cfg.CloudMode {
				if err := w.sendOpportunityEmail(ctx, target, emailChannel, opportunity, result); err != nil {
					return err
				}
			} else {
				limits, ok := billing.Plan(w.planForOrg(ctx, target.OrganizationID))
				if !ok {
					limits = billing.Entitlements{DailyAlerts: 20}
				}
				if err := w.sendOpportunityEmail(ctx, target, emailChannel, opportunity, result); err != nil {
					return err
				}
				allowed, _, err := w.store.ConsumeUsage(ctx, target.OrganizationID, now.Format("2006-01-02"), "daily_alert", limits.DailyAlerts)
				if err != nil {
					return err
				}
				if !allowed {
					return fmt.Errorf("daily alert quota exceeded after delivery")
				}
			}
		}

		if target.Preferences.Push && len(target.PushTokens) > 0 {
			sent, err := w.store.NotificationAlertSent(ctx, target.OrganizationID, "push", opportunity.ID)
			if err != nil {
				return err
			}
			if sent {
				result.SkippedDedup++
				continue
			}
			message := Message{
				ID:             "push-" + opportunity.ID,
				OrganizationID: target.OrganizationID,
				OpportunityID:  opportunity.ID,
				Title:          "Nova licitação no radar",
				Body:           truncate(opportunity.Object, 160),
				SourceURL:      opportunity.SourceURL,
			}
			var lastProviderID string
			var sendErr error
			for _, token := range target.PushTokens {
				pushChannel := Channel{
					Type:           ExpoPush,
					OrganizationID: target.OrganizationID,
					Destination:    token,
					Enabled:        true,
					ConsentedAt:    now,
				}
				lastProviderID, sendErr = w.dispatch.Send(ctx, pushChannel, message)
				if sendErr != nil {
					attempts, recordErr := w.store.RecordNotificationFailure(ctx, target.OrganizationID, "push", opportunity.ID, token, sendErr.Error())
					if recordErr != nil {
						return recordErr
					}
					result.Errors++
					if attempts >= 5 {
						result.Suppressed++
					}
					w.log.Warn("push alert failed", "organization_id", target.OrganizationID, "attempts", attempts, "error", sendErr)
					break
				}
			}
			if sendErr == nil {
				dest := target.PushTokens[0]
				if err := w.store.RecordNotificationAlert(ctx, target.OrganizationID, "push", opportunity.ID, dest, lastProviderID); err != nil {
					return err
				}
				result.OpportunitySent++
			}
		}
	}

	if !target.Preferences.DeadlineReminder {
		return nil
	}
	deals, err := w.store.Deals(ctx, target.OrganizationID)
	if err != nil {
		return err
	}
	for _, deal := range deals {
		if !domain.DealFollowUpDue(deal, now) {
			continue
		}
		dedupe := deal.ID + ":" + deal.NextFollowUpAt.Format(time.RFC3339)
		sent, err := w.store.NotificationAlertSent(ctx, target.OrganizationID, "followup", dedupe)
		if err != nil || sent {
			if sent {
				result.SkippedDedup++
			}
			continue
		}
		if !target.Preferences.Email {
			continue
		}
		message := Message{
			ID:             deal.ID,
			OrganizationID: target.OrganizationID,
			Title:          fmt.Sprintf("[LicitaLens] Follow-up comercial · %s", deal.Title),
			Body:           fmt.Sprintf("Retomar negociação com %s.\nEtapa: %s\nÚltima nota: %s", strings.TrimSpace(deal.BuyerName), deal.Stage, strings.TrimSpace(deal.LastFollowUpNote)),
		}
		providerID, err := w.dispatch.Send(ctx, emailChannel, message)
		if err != nil {
			attempts, recordErr := w.store.RecordNotificationFailure(ctx, target.OrganizationID, "followup", dedupe, target.OwnerEmail, err.Error())
			if recordErr != nil {
				return recordErr
			}
			result.Errors++
			if attempts >= 5 {
				result.Suppressed++
			}
			continue
		}
		if err := w.store.RecordNotificationAlert(ctx, target.OrganizationID, "followup", dedupe, target.OwnerEmail, providerID); err != nil {
			return err
		}
		result.FollowUpSent++
	}
	if err := w.store.AdvanceNotificationOpportunityCursor(ctx, target.OrganizationID, now); err != nil {
		return fmt.Errorf("advance opportunity cursor: %w", err)
	}
	return nil
}

func (w *Worker) shouldAlertOpportunity(profiles []domain.CommercialProfile, opportunity domain.Opportunity) bool {
	for _, profile := range profiles {
		match, ok := MatchOpportunity(profile, opportunity)
		if ok && match.Score >= w.cfg.MinMatchScore {
			return true
		}
	}
	return false
}

func (w *Worker) sendOpportunityEmail(ctx context.Context, target domain.NotificationTarget, emailChannel Channel, opportunity domain.Opportunity, result *domain.NotificationRunResult) error {
	message := Message{
		ID:             opportunity.ID,
		OrganizationID: target.OrganizationID,
		OpportunityID:  opportunity.ID,
		Title:          fmt.Sprintf("[LicitaLens] Nova oportunidade · %s", target.OrganizationName),
		Body:           formatOpportunityBody(opportunity),
		SourceURL:      opportunity.SourceURL,
	}
	providerID, err := w.dispatch.Send(ctx, emailChannel, message)
	if err != nil {
		attempts, recordErr := w.store.RecordNotificationFailure(ctx, target.OrganizationID, "opportunity", opportunity.ID, target.OwnerEmail, err.Error())
		if recordErr != nil {
			return recordErr
		}
		result.Errors++
		if attempts >= 5 {
			result.Suppressed++
		}
		w.log.Warn("email alert failed", "organization_id", target.OrganizationID, "attempts", attempts, "error", err)
		return err
	}
	if err := w.store.RecordNotificationAlert(ctx, target.OrganizationID, "opportunity", opportunity.ID, target.OwnerEmail, providerID); err != nil {
		return err
	}
	result.OpportunitySent++
	return nil
}

func formatOpportunityBody(opportunity domain.Opportunity) string {
	lines := []string{
		opportunity.Object,
		fmt.Sprintf("Órgão: %s", opportunity.OrganizationName),
		fmt.Sprintf("UF: %s", opportunity.State),
	}
	if !opportunity.ProposalDeadline.IsZero() {
		lines = append(lines, fmt.Sprintf("Prazo: %s", opportunity.ProposalDeadline.Format("02/01/2006 15:04")))
	}
	if opportunity.SourceURL != "" {
		lines = append(lines, "Fonte: "+opportunity.SourceURL)
	}
	return strings.Join(lines, "\n")
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max-1] + "…"
}

func (w *Worker) planForOrg(ctx context.Context, organizationID string) string {
	sub, err := w.store.Subscription(ctx, organizationID)
	if err != nil {
		return "essential"
	}
	return sub.Plan
}

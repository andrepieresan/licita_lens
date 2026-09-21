package ingestion

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"licitalens.dev/backend/internal/domain"
	"licitalens.dev/backend/internal/providers/pncp"
)

type Archive interface {
	Put(ctx context.Context, key string, body []byte) error
}
type Publisher interface {
	Publish(ctx context.Context, topic, key string, body []byte) error
}
type CheckpointStore interface {
	IngestionCheckpoint(context.Context, string, string) (string, error)
	SaveIngestionCheckpoint(context.Context, string, string, string) error
}

type Runner struct {
	Client      *pncp.Client
	Archive     Archive
	Publisher   Publisher
	Logger      *slog.Logger
	PageSize    int
	MaxPages    int
	MaxAttempts int
	RetryDelay  time.Duration
	Checkpoints CheckpointStore
}

type Result struct{ Pages, Records int }

func (r Runner) SyncDay(ctx context.Context, day time.Time, modality int) (Result, error) {
	pageNumber, result := 1, Result{}
	for {
		page, err := r.publications(ctx, day, modality, pageNumber)
		if err != nil {
			return result, err
		}
		if err := r.processPage(ctx, day, modality, pageNumber, page); err != nil {
			return result, err
		}
		result.Pages++
		result.Records += len(page.Opportunities)
		if page.Remaining == 0 || len(page.Opportunities) == 0 || (r.MaxPages > 0 && result.Pages >= r.MaxPages) {
			break
		}
		pageNumber++
	}
	r.Logger.Info("ingestion checkpoint", "source", "pncp", "day", day.Format("2006-01-02"), "modality", modality, "pages", result.Pages, "records", result.Records)
	return result, nil
}

// SyncRecent reads the newest pages for a day. PNCP filters publications by
// date, not timestamp, and exposes the newest records at the end.
func (r Runner) SyncRecent(ctx context.Context, day time.Time, modality, lookbackPages int) (Result, error) {
	if lookbackPages < 1 {
		lookbackPages = 1
	}
	probe, err := r.publications(ctx, day, modality, 1)
	if err != nil {
		return Result{}, err
	}
	start := probe.TotalPages - lookbackPages + 1
	if start < 1 {
		start = 1
	}
	partition := fmt.Sprintf("%s:%d", day.Format("2006-01-02"), modality)
	if r.Checkpoints != nil {
		if cursor, err := r.Checkpoints.IngestionCheckpoint(ctx, "pncp", partition); err != nil {
			return Result{}, err
		} else if page, parseErr := strconv.Atoi(cursor); parseErr == nil && page >= start && page < probe.TotalPages {
			start = page + 1
		}
	}
	result := Result{}
	for pageNumber := start; pageNumber <= probe.TotalPages; pageNumber++ {
		page := probe
		if pageNumber != 1 {
			page, err = r.publications(ctx, day, modality, pageNumber)
			if err != nil {
				return result, err
			}
		}
		if err := r.processPage(ctx, day, modality, pageNumber, page); err != nil {
			return result, err
		}
		result.Pages++
		result.Records += len(page.Opportunities)
		if r.Checkpoints != nil {
			if err := r.Checkpoints.SaveIngestionCheckpoint(ctx, "pncp", partition, strconv.Itoa(pageNumber)); err != nil {
				return result, err
			}
		}
	}
	r.Logger.Info("incremental checkpoint", "source", "pncp", "day", day.Format("2006-01-02"), "modality", modality, "pages", result.Pages, "records", result.Records)
	return result, nil
}

func (r Runner) publications(ctx context.Context, day time.Time, modality, pageNumber int) (pncp.Page, error) {
	attempts := r.MaxAttempts
	if attempts < 1 {
		attempts = 3
	}
	delay := r.RetryDelay
	if delay <= 0 {
		delay = 500 * time.Millisecond
	}
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		page, err := r.Client.Publications(ctx, day, day, modality, pageNumber, r.PageSize)
		if err == nil {
			return page, nil
		}
		lastErr = err
		if attempt == attempts || ctx.Err() != nil {
			break
		}
		r.Logger.Warn("PNCP request failed; retrying", "page", pageNumber, "modality", modality, "attempt", attempt, "max_attempts", attempts, "error", err)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return pncp.Page{}, ctx.Err()
		case <-timer.C:
		}
		if delay < 30*time.Second {
			delay *= 2
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
		}
	}
	return pncp.Page{}, fmt.Errorf("PNCP page %d failed after %d attempt(s): %w", pageNumber, attempts, lastErr)
}

func (r Runner) processPage(ctx context.Context, day time.Time, modality, pageNumber int, page pncp.Page) error {
	hash := sha256.Sum256(page.Raw)
	archiveKey := fmt.Sprintf("pncp/publications/%s/modality=%d/page=%d/%s.json", day.Format("2006-01-02"), modality, pageNumber, hex.EncodeToString(hash[:]))
	if err := r.Archive.Put(ctx, archiveKey, page.Raw); err != nil {
		return fmt.Errorf("archive raw page: %w", err)
	}
	for _, opportunity := range page.Opportunities {
		body, _ := json.Marshal(event[domain.Opportunity]{ID: opportunity.SourceID + ":" + opportunity.UpdatedAt.Format(time.RFC3339), Type: "procurement.discovered.v1", OccurredAt: time.Now().UTC(), Data: opportunity})
		if err := r.Publisher.Publish(ctx, "procurement.discovered.v1", opportunity.SourceID, body); err != nil {
			return fmt.Errorf("publish opportunity: %w", err)
		}
	}
	return nil
}

type event[T any] struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       T         `json:"data"`
}

type FileArchive struct{ Root string }

func (a FileArchive) Put(_ context.Context, key string, body []byte) error {
	path := filepath.Join(a.Root, filepath.Clean(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o640)
}

type JSONPublisher struct{ Output *os.File }

func (p JSONPublisher) Publish(_ context.Context, topic, key string, body []byte) error {
	return json.NewEncoder(p.Output).Encode(map[string]any{"topic": topic, "key": key, "body": json.RawMessage(body)})
}

package ingestion

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"licitalens.dev/backend/internal/providers/pncp"
)

type archiveSpy struct{ calls int }

func (a *archiveSpy) Put(context.Context, string, []byte) error { a.calls++; return nil }

type publisherSpy struct{ calls int }

func (p *publisherSpy) Publish(context.Context, string, string, []byte) error { p.calls++; return nil }

type checkpointSpy struct {
	cursor string
	saves  []string
}

func (c *checkpointSpy) IngestionCheckpoint(context.Context, string, string) (string, error) {
	return c.cursor, nil
}
func (c *checkpointSpy) SaveIngestionCheckpoint(_ context.Context, _, _, cursor string) error {
	c.cursor = cursor
	c.saves = append(c.saves, cursor)
	return nil
}

func TestRunnerArchivesBeforePublishing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"numeroControlePNCP":"id","objetoCompra":"item"}],"totalPaginas":1,"numeroPagina":1,"paginasRestantes":0}`))
	}))
	defer server.Close()
	a, p := &archiveSpy{}, &publisherSpy{}
	r := Runner{Client: pncp.NewClient(server.URL, time.Second), Archive: a, Publisher: p, Logger: testLogger(), PageSize: 10}
	result, err := r.SyncDay(context.Background(), time.Now(), 6)
	if err != nil {
		t.Fatal(err)
	}
	if a.calls != 1 || p.calls != 1 || result.Records != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestSyncRecentResumesAfterLastPersistedPage(t *testing.T) {
	pages := []int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("pagina"))
		pages = append(pages, page)
		remaining := 3 - page
		_, _ = w.Write([]byte(`{"data":[{"numeroControlePNCP":"id-` + strconv.Itoa(page) + `","objetoCompra":"item"}],"totalPaginas":3,"numeroPagina":` + strconv.Itoa(page) + `,"paginasRestantes":` + strconv.Itoa(remaining) + `}`))
	}))
	defer server.Close()
	checkpoint := &checkpointSpy{cursor: "1"}
	archive, publisher := &archiveSpy{}, &publisherSpy{}
	runner := Runner{Client: pncp.NewClient(server.URL, time.Second), Archive: archive, Publisher: publisher, Logger: testLogger(), PageSize: 10, Checkpoints: checkpoint}
	result, err := runner.SyncRecent(context.Background(), time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), 6, 3)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pages != 2 || checkpoint.cursor != "3" || len(checkpoint.saves) != 2 {
		t.Fatalf("resume failed: result=%#v checkpoint=%#v pages=%v", result, checkpoint, pages)
	}
	if len(pages) != 3 || pages[0] != 1 || pages[1] != 2 || pages[2] != 3 {
		t.Fatalf("unexpected pages: %v", pages)
	}
}

func TestSyncRecentRetriesTransientPNCPFailureBeforeProcessing(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "temporary", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"numeroControlePNCP":"retry-id","objetoCompra":"item"}],"totalPaginas":1,"numeroPagina":1,"paginasRestantes":0}`))
	}))
	defer server.Close()
	archive, publisher := &archiveSpy{}, &publisherSpy{}
	runner := Runner{Client: pncp.NewClient(server.URL, time.Second), Archive: archive, Publisher: publisher, Logger: testLogger(), PageSize: 10, MaxAttempts: 3, RetryDelay: time.Millisecond}
	result, err := runner.SyncRecent(context.Background(), time.Now(), 6, 1)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || result.Records != 1 || archive.calls != 1 || publisher.calls != 1 {
		t.Fatalf("unexpected retry result: attempts=%d result=%#v archive=%d publish=%d", attempts, result, archive.calls, publisher.calls)
	}
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

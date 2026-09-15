package ingestion

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"licitalens.dev/backend/internal/providers/pncp"
)

type archiveSpy struct{ calls int }

func (a *archiveSpy) Put(context.Context, string, []byte) error { a.calls++; return nil }

type publisherSpy struct{ calls int }

func (p *publisherSpy) Publish(context.Context, string, string, []byte) error { p.calls++; return nil }

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

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(os.Stderr, nil)) }

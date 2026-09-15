package pncp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPublicationsMapsOfficialPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"numeroControlePNCP":"123-1-1/2026","objetoCompra":"Notebooks","modalidadeId":6,"valorTotalEstimado":10.25,"dataPublicacaoPncp":"2026-09-01T10:00:00","dataAtualizacaoGlobal":"2026-09-01T10:01:00","dataEncerramentoProposta":"2026-09-20T10:00:00","anoCompra":2026,"sequencialCompra":1,"orgaoEntidade":{"cnpj":"123","razaoSocial":"Órgão"},"unidadeOrgao":{"ufSigla":"PR","municipioNome":"Curitiba"}}],"totalPaginas":1,"numeroPagina":1,"paginasRestantes":0}`))
	}))
	defer server.Close()
	page, err := NewClient(server.URL, time.Second).Publications(context.Background(), time.Now(), time.Now(), 6, 1, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Opportunities) != 1 || page.Opportunities[0].EstimatedValueCents != 1025 {
		t.Fatalf("unexpected page: %#v", page)
	}
}

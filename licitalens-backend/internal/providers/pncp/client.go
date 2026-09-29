package pncp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"licitalens.dev/backend/internal/domain"
)

const defaultBaseURL = "https://pncp.gov.br/api/consulta/v1"

type Client struct {
	baseURL        string
	fallbackURL    string
	fallbackAPIKey string
	http           *http.Client
}

type Page struct {
	Opportunities []domain.Opportunity
	Number        int
	TotalPages    int
	Remaining     int
	Raw           []byte
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return NewClientWithFallback(baseURL, "", "", timeout)
}

func NewClientWithFallback(baseURL, fallbackURL, fallbackAPIKey string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{baseURL: baseURL, fallbackURL: fallbackURL, fallbackAPIKey: fallbackAPIKey, http: &http.Client{Timeout: timeout}}
}

func (c *Client) Publications(ctx context.Context, from, to time.Time, modality, page, pageSize int) (Page, error) {
	result, err := c.publications(ctx, c.baseURL, "", from, to, modality, page, pageSize)
	if err == nil || c.fallbackURL == "" || ctx.Err() != nil || !isRetryable(err) {
		return result, err
	}
	fallback, fallbackErr := c.publications(ctx, c.fallbackURL, c.fallbackAPIKey, from, to, modality, page, pageSize)
	if fallbackErr != nil {
		return Page{}, fmt.Errorf("pncp primary failed: %v; fallback failed: %w", err, fallbackErr)
	}
	return fallback, nil
}

func (c *Client) publications(ctx context.Context, baseURL, apiKey string, from, to time.Time, modality, page, pageSize int) (Page, error) {
	if isOfficialOpenData(baseURL) {
		return c.openDataPublications(ctx, baseURL, from, to, modality, page, pageSize)
	}
	query := url.Values{}
	query.Set("dataInicial", from.Format("20060102"))
	query.Set("dataFinal", to.Format("20060102"))
	query.Set("codigoModalidadeContratacao", strconv.Itoa(modality))
	query.Set("pagina", strconv.Itoa(page))
	query.Set("tamanhoPagina", strconv.Itoa(pageSize))
	endpoint := baseURL + "/contratacoes/publicacao?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LicitaLens/0.1 (+https://licitalens.dev)")
	if apiKey != "" {
		req.Header.Set("x-api-key", apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return Page{}, requestError{err: fmt.Errorf("pncp request: %w", err), retryable: true}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Page{}, requestError{err: fmt.Errorf("read pncp response: %w", err), retryable: true}
	}
	if resp.StatusCode/100 != 2 {
		return Page{}, requestError{err: fmt.Errorf("pncp returned %d: %s", resp.StatusCode, string(raw)), retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500}
	}

	var payload response
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Page{}, fmt.Errorf("decode pncp response: %w", err)
	}
	result := Page{Number: payload.Number, TotalPages: payload.TotalPages, Remaining: payload.Remaining, Raw: raw}
	for _, item := range payload.Data {
		result.Opportunities = append(result.Opportunities, item.opportunity())
	}
	return result, nil
}

func isOfficialOpenData(baseURL string) bool {
	parsed, err := url.Parse(baseURL)
	return err == nil && parsed.Hostname() == "dadosabertos.compras.gov.br"
}

func (c *Client) openDataPublications(ctx context.Context, baseURL string, from, to time.Time, modality, page, pageSize int) (Page, error) {
	query := url.Values{}
	query.Set("dataPublicacaoPncpInicial", from.Format("2006-01-02"))
	query.Set("dataPublicacaoPncpFinal", to.Format("2006-01-02"))
	query.Set("codigoModalidade", strconv.Itoa(modality))
	query.Set("pagina", strconv.Itoa(page))
	query.Set("tamanhoPagina", strconv.Itoa(pageSize))
	endpoint := strings.TrimRight(baseURL, "/") + "/modulo-contratacoes/1_consultarContratacoes_PNCP_14133?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LicitaLens/0.1 (+https://licitalens.dev)")
	resp, err := c.http.Do(req)
	if err != nil {
		return Page{}, requestError{err: fmt.Errorf("PNCP dados abertos request: %w", err), retryable: true}
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Page{}, requestError{err: fmt.Errorf("read PNCP dados abertos response: %w", err), retryable: true}
	}
	if resp.StatusCode/100 != 2 {
		return Page{}, requestError{err: fmt.Errorf("PNCP dados abertos returned %d: %s", resp.StatusCode, string(raw)), retryable: resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500}
	}
	var payload openDataResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Page{}, fmt.Errorf("decode PNCP dados abertos response: %w", err)
	}
	result := Page{Number: page, TotalPages: payload.TotalPages, Remaining: payload.Remaining, Raw: raw}
	for _, item := range payload.Data {
		result.Opportunities = append(result.Opportunities, item.publication().opportunity())
	}
	return result, nil
}

type requestError struct {
	err       error
	retryable bool
}

func (e requestError) Error() string { return e.err.Error() }
func (e requestError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var requestErr requestError
	return errors.As(err, &requestErr) && requestErr.retryable
}

type response struct {
	Data       []publication `json:"data"`
	TotalPages int           `json:"totalPaginas"`
	Number     int           `json:"numeroPagina"`
	Remaining  int           `json:"paginasRestantes"`
}

type openDataResponse struct {
	Data       []openDataPublication `json:"resultado"`
	TotalPages int                   `json:"totalPaginas"`
	Remaining  int                   `json:"paginasRestantes"`
}

type openDataPublication struct {
	ControlNumber    string  `json:"numeroControlePNCP"`
	Object           string  `json:"objetoCompra"`
	Modality         int     `json:"codigoModalidade"`
	Estimated        float64 `json:"valorTotalEstimado"`
	Published        string  `json:"dataPublicacaoPncp"`
	Updated          string  `json:"dataAtualizacaoPncp"`
	Deadline         string  `json:"dataEncerramentoPropostaPncp"`
	Year             int     `json:"anoCompraPncp"`
	Sequence         int     `json:"sequencialCompraPncp"`
	OrganizationCNPJ string  `json:"orgaoEntidadeCnpj"`
	OrganizationName string  `json:"orgaoEntidadeRazaoSocial"`
	State            string  `json:"unidadeOrgaoUfSigla"`
	Municipality     string  `json:"unidadeOrgaoMunicipioNome"`
}

func (p openDataPublication) publication() publication {
	return publication{ControlNumber: p.ControlNumber, Object: p.Object, Modality: p.Modality, Estimated: p.Estimated, Published: p.Published, Updated: p.Updated, Deadline: p.Deadline, Year: p.Year, Sequence: p.Sequence, OrganizationRaw: struct {
		CNPJ string `json:"cnpj"`
		Name string `json:"razaoSocial"`
	}{CNPJ: p.OrganizationCNPJ, Name: p.OrganizationName}, Unit: struct {
		State        string `json:"ufSigla"`
		Municipality string `json:"municipioNome"`
	}{State: p.State, Municipality: p.Municipality}}
}

type publication struct {
	ControlNumber   string                      `json:"numeroControlePNCP"`
	Object          string                      `json:"objetoCompra"`
	Modality        int                         `json:"modalidadeId"`
	Estimated       float64                     `json:"valorTotalEstimado"`
	Published       string                      `json:"dataPublicacaoPncp"`
	Updated         string                      `json:"dataAtualizacaoGlobal"`
	Deadline        string                      `json:"dataEncerramentoProposta"`
	Year            int                         `json:"anoCompra"`
	Sequence        int                         `json:"sequencialCompra"`
	Organization    struct{ CNPJ, Name string } `json:"-"`
	OrganizationRaw struct {
		CNPJ string `json:"cnpj"`
		Name string `json:"razaoSocial"`
	} `json:"orgaoEntidade"`
	Unit struct {
		State        string `json:"ufSigla"`
		Municipality string `json:"municipioNome"`
	} `json:"unidadeOrgao"`
}

func (p publication) opportunity() domain.Opportunity {
	return domain.Opportunity{
		ID: p.ControlNumber, Source: "pncp", SourceID: p.ControlNumber, Object: p.Object,
		OrganizationName: p.OrganizationRaw.Name, State: p.Unit.State, Municipality: p.Unit.Municipality,
		ModalityCode: p.Modality, EstimatedValueCents: int64(p.Estimated*100 + .5),
		PublishedAt: parseTime(p.Published), ProposalDeadline: parseTime(p.Deadline), UpdatedAt: parseTime(p.Updated),
		SourceURL: fmt.Sprintf("https://pncp.gov.br/app/editais/%s/%d/%d", p.OrganizationRaw.CNPJ, p.Year, p.Sequence),
	}
}

func parseTime(value string) time.Time {
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

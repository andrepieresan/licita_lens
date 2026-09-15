package pncp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"licitalens.dev/backend/internal/domain"
)

const defaultBaseURL = "https://pncp.gov.br/api/consulta/v1"

type Client struct {
	baseURL string
	http    *http.Client
}

type Page struct {
	Opportunities []domain.Opportunity
	Number        int
	TotalPages    int
	Remaining     int
	Raw           []byte
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: timeout}}
}

func (c *Client) Publications(ctx context.Context, from, to time.Time, modality, page, pageSize int) (Page, error) {
	query := url.Values{}
	query.Set("dataInicial", from.Format("20060102"))
	query.Set("dataFinal", to.Format("20060102"))
	query.Set("codigoModalidadeContratacao", strconv.Itoa(modality))
	query.Set("pagina", strconv.Itoa(page))
	query.Set("tamanhoPagina", strconv.Itoa(pageSize))
	endpoint := c.baseURL + "/contratacoes/publicacao?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Page{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LicitaLens/0.1 (+https://licitalens.dev)")
	resp, err := c.http.Do(req)
	if err != nil {
		return Page{}, fmt.Errorf("pncp request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return Page{}, fmt.Errorf("read pncp response: %w", err)
	}
	if resp.StatusCode/100 != 2 {
		return Page{}, fmt.Errorf("pncp returned %d: %s", resp.StatusCode, string(raw))
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

type response struct {
	Data       []publication `json:"data"`
	TotalPages int           `json:"totalPaginas"`
	Number     int           `json:"numeroPagina"`
	Remaining  int           `json:"paginasRestantes"`
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
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

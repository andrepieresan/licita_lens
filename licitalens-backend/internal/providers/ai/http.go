package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPProvider struct {
	provider, baseURL, apiKey, embeddingModel, textModel string
	client                                               *http.Client
}

func NewOpenAI(apiKey, embeddingModel, textModel string) *HTTPProvider {
	if embeddingModel == "" {
		embeddingModel = "text-embedding-3-small"
	}
	if textModel == "" {
		textModel = "gpt-5-mini"
	}
	return &HTTPProvider{provider: "openai", baseURL: "https://api.openai.com", apiKey: apiKey, embeddingModel: embeddingModel, textModel: textModel, client: &http.Client{Timeout: 45 * time.Second}}
}

func NewOllama(baseURL, embeddingModel, textModel string) *HTTPProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if embeddingModel == "" {
		embeddingModel = "embeddinggemma"
	}
	if textModel == "" {
		textModel = "gemma3"
	}
	return &HTTPProvider{provider: "ollama", baseURL: strings.TrimRight(baseURL, "/"), embeddingModel: embeddingModel, textModel: textModel, client: &http.Client{Timeout: 90 * time.Second}}
}

func (p *HTTPProvider) Name() string { return p.provider }

func (p *HTTPProvider) Embed(ctx context.Context, texts []string) ([][]float32, Usage, error) {
	if p.provider == "openai" {
		var out struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
			Usage struct {
				Prompt int `json:"prompt_tokens"`
				Total  int `json:"total_tokens"`
			} `json:"usage"`
		}
		err := p.post(ctx, "/v1/embeddings", map[string]any{"model": p.embeddingModel, "input": texts}, &out)
		vectors := make([][]float32, len(out.Data))
		for i := range out.Data {
			vectors[i] = out.Data[i].Embedding
		}
		return vectors, Usage{InputTokens: out.Usage.Total}, err
	}
	var out struct {
		Embeddings [][]float32 `json:"embeddings"`
		Tokens     int         `json:"prompt_eval_count"`
	}
	err := p.post(ctx, "/api/embed", map[string]any{"model": p.embeddingModel, "input": texts}, &out)
	return out.Embeddings, Usage{InputTokens: out.Tokens}, err
}

func (p *HTTPProvider) Explain(ctx context.Context, instructions, evidence string) (Explanation, error) {
	if p.provider == "openai" {
		var out struct {
			Output []struct {
				Content []struct{ Type, Text string } `json:"content"`
			} `json:"output"`
			Usage struct {
				Input  int `json:"input_tokens"`
				Output int `json:"output_tokens"`
			} `json:"usage"`
		}
		err := p.post(ctx, "/v1/responses", map[string]any{"model": p.textModel, "instructions": instructions, "input": evidence, "max_output_tokens": 350, "store": false}, &out)
		if err != nil {
			return Explanation{}, err
		}
		var parts []string
		for _, item := range out.Output {
			for _, content := range item.Content {
				if content.Type == "output_text" {
					parts = append(parts, content.Text)
				}
			}
		}
		return Explanation{Text: strings.Join(parts, "\n"), Usage: Usage{InputTokens: out.Usage.Input, OutputTokens: out.Usage.Output}}, nil
	}
	var out struct {
		Response string `json:"response"`
		Input    int    `json:"prompt_eval_count"`
		Output   int    `json:"eval_count"`
	}
	err := p.post(ctx, "/api/generate", map[string]any{"model": p.textModel, "system": instructions, "prompt": evidence, "stream": false}, &out)
	return Explanation{Text: out.Response, Usage: Usage{InputTokens: out.Input, OutputTokens: out.Output}}, err
}

func (p *HTTPProvider) post(ctx context.Context, path string, payload, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s returned %d: %s", p.provider, resp.StatusCode, string(raw))
	}
	if err := json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("decode %s response: %w", p.provider, err)
	}
	return nil
}

package ai

import "context"

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type Explanation struct {
	Text  string `json:"text"`
	Usage Usage  `json:"usage"`
}

type Provider interface {
	Name() string
	Embed(ctx context.Context, texts []string) ([][]float32, Usage, error)
	Explain(ctx context.Context, instructions, evidence string) (Explanation, error)
}

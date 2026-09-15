package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type ExpoPushProvider struct {
	client      *http.Client
	accessToken string
}

func NewExpoPushProvider(accessToken string) *ExpoPushProvider {
	return &ExpoPushProvider{client: http.DefaultClient, accessToken: accessToken}
}

func (p *ExpoPushProvider) Type() ChannelType { return ExpoPush }

type expoPushMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title,omitempty"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
}

type expoPushResponse struct {
	Data []struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Message string `json:"message"`
	} `json:"data"`
}

func (p *ExpoPushProvider) Send(ctx context.Context, channel Channel, message Message) (string, error) {
	token := strings.TrimSpace(channel.Destination)
	if token == "" {
		return "", fmt.Errorf("expo push token missing")
	}
	payload, err := json.Marshal(expoPushMessage{
		To:    token,
		Title: message.Title,
		Body:  message.Body,
		Data: map[string]string{
			"opportunity_id": message.OpportunityID,
			"source_url":     message.SourceURL,
		},
	})
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://exp.host/--/api/v2/push/send", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if p.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+p.accessToken)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("expo push HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed expoPushResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", err
	}
	if len(parsed.Data) == 0 {
		return "", fmt.Errorf("expo push empty response")
	}
	item := parsed.Data[0]
	if item.Status != "ok" {
		return "", fmt.Errorf("expo push rejected: %s", item.Message)
	}
	if item.ID != "" {
		return item.ID, nil
	}
	return "expo-sent", nil
}

// Copyright 2024 ff, Scalytics, Inc. - https://www.scalytics.io
// SPDX-License-Identifier: Apache-2.0

package vet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/scalytics/kafSIEM/internal/collector/config"
)

type Client struct {
	httpClient  *http.Client
	baseURL     string
	apiKey      string
	model       string
	provider    string
	temperature float64
}

func NewClient(cfg config.Config) *Client {
	timeout := time.Duration(cfg.VettingTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 45 * time.Second
	}
	return &Client{
		httpClient:  &http.Client{Timeout: timeout},
		baseURL:     strings.TrimSpace(cfg.VettingBaseURL),
		apiKey:      strings.TrimSpace(cfg.VettingAPIKey),
		model:       strings.TrimSpace(cfg.VettingModel),
		provider:    strings.TrimSpace(cfg.VettingProvider),
		temperature: cfg.VettingTemperature,
	}
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (c *Client) Complete(ctx context.Context, messages []Message) (string, error) {
	reqBody, err := json.Marshal(chatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
	})
	if err != nil {
		return "", fmt.Errorf("marshal source vetting request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, completionsURL(c.baseURL), bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("build source vetting request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if c.provider != "" {
		req.Header.Set("X-EUOSINT-Provider", c.provider)
	}

	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request source vetting completion: %w", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("read source vetting response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("source vetting endpoint status %d: %s", res.StatusCode, strings.TrimSpace(string(body)))
	}

	var parsed chatResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decode source vetting response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("source vetting response returned no choices")
	}
	return strings.TrimSpace(parsed.Choices[0].Message.Content), nil
}

func completionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return "https://api.openai.com/v1/chat/completions"
	}
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

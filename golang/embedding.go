package golang

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func GetEmbedding(ctx context.Context, baseURL, model, text string) ([]float32, error) {
	reqBody := struct {
		Model  string `json:"model"`
		Prompt string `json:"prompt"`
	}{
		Model:  model,
		Prompt: text,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/embeddings", bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post to ollama: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama error: %s: %s", res.Status, string(body))
	}

	var resp struct {
		Embedding []float32 `json:"embedding"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return resp.Embedding, nil
}

package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type QdrantClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewQdrantClient() *QdrantClient {
	baseURL := os.Getenv("QDRANT_URL")
	if baseURL == "" {
		baseURL = "http://localhost:6333"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	apiKey := os.Getenv("QDRANT_API_KEY")

	return &QdrantClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *QdrantClient) Name() string { return "qdrant" }

func (c *QdrantClient) do(method, path string, body, respTarget interface{}) error {
	url := c.baseURL + path
	var bodyReader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("api-key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("Qdrant error (status %d): %s", resp.StatusCode, string(respData))
	}

	if respTarget != nil {
		var wrapper struct {
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(respData, &wrapper); err != nil {
			return fmt.Errorf("parse: %w", err)
		}
		if err := json.Unmarshal(wrapper.Result, respTarget); err != nil {
			return fmt.Errorf("unmarshal result: %w", err)
		}
	}

	return nil
}

func (c *QdrantClient) Upsert(ctx context.Context, collection string, points []Point) error {
	type qPoint struct {
		ID      string                 `json:"id"`
		Vector  []float32              `json:"vector"`
		Payload map[string]interface{} `json:"payload"`
	}

	var qPoints []qPoint
	for _, p := range points {
		qPoints = append(qPoints, qPoint{
			ID:      p.ID,
			Vector:  p.Vector,
			Payload: p.Payload,
		})
	}

	body := map[string]interface{}{
		"points": qPoints,
	}

	return c.do("PUT", fmt.Sprintf("/collections/%s/points", collection), body, nil)
}

func (c *QdrantClient) Search(ctx context.Context, collection string, vector []float32, limit int) ([]Point, error) {
	body := map[string]interface{}{
		"vector": vector,
		"limit":  limit,
		"with_payload": true,
	}

	var qResult []struct {
		ID      string                 `json:"id"`
		Score   float64                `json:"score"`
		Payload map[string]interface{} `json:"payload"`
	}

	if err := c.do("POST", fmt.Sprintf("/collections/%s/points/search", collection), body, &qResult); err != nil {
		return nil, err
	}

	var points []Point
	for _, r := range qResult {
		points = append(points, Point{
			ID:      r.ID,
			Score:   r.Score,
			Payload: r.Payload,
		})
	}

	return points, nil
}

func (c *QdrantClient) DeleteCollection(ctx context.Context, collection string) error {
	return c.do("DELETE", fmt.Sprintf("/collections/%s", collection), nil, nil)
}

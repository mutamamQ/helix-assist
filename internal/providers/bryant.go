package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/leona/helix-assist/internal/lsp"
	"github.com/leona/helix-assist/internal/util"
)

// BryantProvider talks to the local BryantGPT loopback bridge (bryant-provider),
// which exposes an OpenAI-compatible /v1/chat/completions endpoint.
// Model IDs look like "15d8cc8844/OPENAI_GPT6_LUNA".
type BryantProvider struct {
	apiKey    string
	model     string
	chatModel string
	endpoint  string
	timeout   time.Duration
	logger    *lsp.Logger
	client    *http.Client
}

func NewBryantProvider(apiKey, model, chatModel, endpoint string, timeoutMs int, logger *lsp.Logger) *BryantProvider {
	if chatModel == "" {
		chatModel = model
	}
	return &BryantProvider{
		apiKey:    apiKey,
		model:     model,
		chatModel: chatModel,
		endpoint:  strings.TrimSuffix(endpoint, "/"),
		timeout:   time.Duration(timeoutMs) * time.Millisecond,
		logger:    logger,
		client:    &http.Client{},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature *float64      `json:"temperature,omitempty"`
	Stream      bool          `json:"stream"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *BryantProvider) Completion(ctx context.Context, req CompletionRequest, filepath, languageID string, numSuggestions int) ([]string, error) {
	if numSuggestions < 1 {
		numSuggestions = 1
	}
	system := BuildCompletionSystemPrompt(languageID)
	user := BuildCompletionUserPrompt(filepath, req.ContentBefore, req.ContentAfter)

	temp := 0.0
	if numSuggestions > 1 {
		temp = 0.4
	}

	// Fire suggestions in parallel; completion latency matters more than cost.
	var wg sync.WaitGroup
	var mu sync.Mutex
	results := make([]string, 0, numSuggestions)
	var firstErr error

	for i := 0; i < numSuggestions; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			text, err := p.chat(ctx, p.model, system, user, 512, &temp)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return
			}
			if text = cleanCodeOutput(text); text != "" {
				results = append(results, text)
			}
		}()
	}
	wg.Wait()

	if len(results) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return util.UniqueStrings(results), nil
}

func (p *BryantProvider) Chat(ctx context.Context, query, content, filepath, languageID string) (*ChatResponse, error) {
	cleanFilepath := strings.TrimPrefix(filepath, "file://")
	system := BuildChatSystemPrompt(languageID)
	user := BuildChatUserPrompt(languageID, cleanFilepath, content, query)

	temp := 0.1
	text, err := p.chat(ctx, p.chatModel, system, user, 8192, &temp)
	if err != nil {
		return nil, err
	}
	text = cleanCodeOutput(text)
	if text == "" {
		return nil, fmt.Errorf("no completion found")
	}
	p.logger.Log("DEBUG [Bryant Chat]: Extracted text:", text)
	return &ChatResponse{Result: text}, nil
}

func (p *BryantProvider) chat(ctx context.Context, model, system, user string, maxTokens int, temp *float64) (string, error) {
	body := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:   maxTokens,
		Temperature: temp,
		Stream:      false,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	// Only impose the provider timeout when the caller didn't set a deadline,
	// so long-running code actions can use their own (longer) budget.
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.endpoint+"/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	start := time.Now()
	resp, err := p.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	p.logger.Log("DEBUG [Bryant]:", model, "status", resp.StatusCode, "in", time.Since(start).String())

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var parsed chatResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("no choices in response")
	}
	return parsed.Choices[0].Message.Content, nil
}

// Raw sends an arbitrary system/user prompt to a specific model ("" = chat model).
func (p *BryantProvider) Raw(ctx context.Context, model, system, user string, maxTokens int) (string, error) {
	if model == "" {
		model = p.chatModel
	}
	temp := 0.2
	return p.chat(ctx, model, system, user, maxTokens, &temp)
}

// CleanCodeOutput is the exported fence stripper used by handlers and hxai.
func CleanCodeOutput(s string) string { return cleanCodeOutput(s) }

// cleanCodeOutput strips a surrounding ``` fence if the model added one anyway.
func cleanCodeOutput(s string) string {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "```") {
		if nl := strings.Index(t, "\n"); nl != -1 {
			t = t[nl+1:]
		} else {
			t = strings.TrimPrefix(t, "```")
		}
		t = strings.TrimSuffix(strings.TrimRight(t, " \n\t"), "```")
		return strings.TrimRight(t, " \n\t")
	}
	return strings.TrimRight(s, " \n\t")
}

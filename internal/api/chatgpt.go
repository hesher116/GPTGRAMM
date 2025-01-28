package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

type ChatGPT struct {
	apiKey string
	model  string
	context []chatMessage
}

const (
	ModelGPT3 = "gpt-3.5-turbo"
	ModelGPT4 = "gpt-4"
)

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func NewChatGPT(apiKey string) *ChatGPT {
	return &ChatGPT{
		apiKey: apiKey,
		model:  ModelGPT3,
	}
}

func (c *ChatGPT) SetModel(model string) {
	c.model = model
}

func (c *ChatGPT) SendMessage(prompt string) (string, error) {
	c.context = append(c.context, chatMessage{Role: "user", Content: prompt})

	if len(c.context) > 10 {
		c.context = c.context[len(c.context)-10:]
	}

	reqBody := chatRequest{
		Model:    c.model,
		Messages: c.context,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("помилка маршалінгу запиту: %w", err)
	}

	log.Printf("Відправляємо запит до OpenAI: %s", string(jsonData))

	req, err := http.NewRequest("POST", "https://api.openai.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("помилка створення запиту: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("помилка виконання запиту: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("помилка API (код %d): %s", resp.StatusCode, string(body))
	}

	var response chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("помилка декодування відповіді: %w", err)
	}

	if len(response.Choices) == 0 {
		return "", fmt.Errorf("порожня відповідь від API")
	}

	c.context = append(c.context, chatMessage{
		Role:    "assistant",
		Content: response.Choices[0].Message.Content,
	})

	return response.Choices[0].Message.Content, nil
}

func (c *ChatGPT) ClearContext() {
	c.context = nil
} 
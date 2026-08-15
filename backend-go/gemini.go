package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// --- Тела запроса/ответа Gemini REST API ---

type gPart struct {
	Text       string      `json:"text,omitempty"`
	InlineData *gInlineData `json:"inline_data,omitempty"`
}

type gInlineData struct {
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
}

type gContent struct {
	Parts []gPart `json:"parts"`
}

type gGenConfig struct {
	ResponseMimeType string          `json:"responseMimeType"`
	ResponseSchema   json.RawMessage `json:"responseSchema"`
	Temperature      float64         `json:"temperature"`
}

type gRequest struct {
	Contents          []gContent  `json:"contents"`
	SystemInstruction *gContent   `json:"system_instruction,omitempty"`
	GenerationConfig  gGenConfig  `json:"generationConfig"`
}

type gResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

var httpClient = &http.Client{Timeout: 5 * time.Minute}

// parseSpecFromPDF отправляет PDF напрямую в Gemini и возвращает разобранную спецификацию.
// Повторяет запрос при временных ошибках (429/503) с нарастающей задержкой.
func parseSpecFromPDF(pdf []byte) (*SpecResult, error) {
	apiKey := getAPIKey()
	if apiKey == "" {
		return nil, fmt.Errorf("не найден GEMINI_API_KEY")
	}

	reqBody := gRequest{
		Contents: []gContent{{
			Parts: []gPart{
				{InlineData: &gInlineData{MimeType: "application/pdf", Data: base64.StdEncoding.EncodeToString(pdf)}},
				{Text: "Извлеки спецификацию оборудования, изделий и материалов из этого чертежа."},
			},
		}},
		SystemInstruction: &gContent{Parts: []gPart{{Text: systemPrompt}}},
		GenerationConfig: gGenConfig{
			ResponseMimeType: "application/json",
			ResponseSchema:   json.RawMessage(specSchema),
			Temperature:      0,
		},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		geminiModel(), apiKey)

	maxRetries := 5
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		resp, err := httpClient.Post(url, "application/json", bytes.NewReader(payload))
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(1<<attempt) * time.Second)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == 429 || resp.StatusCode == 503 {
			lastErr = fmt.Errorf("gemini %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			time.Sleep(time.Duration(1<<attempt) * time.Second)
			continue
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("gemini %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		var gr gResponse
		if err := json.Unmarshal(body, &gr); err != nil {
			return nil, fmt.Errorf("разбор ответа gemini: %w", err)
		}
		if gr.Error != nil {
			return nil, fmt.Errorf("gemini: %s", gr.Error.Message)
		}
		if len(gr.Candidates) == 0 || len(gr.Candidates[0].Content.Parts) == 0 {
			return nil, fmt.Errorf("gemini вернул пустой ответ")
		}

		var spec SpecResult
		if err := json.Unmarshal([]byte(gr.Candidates[0].Content.Parts[0].Text), &spec); err != nil {
			return nil, fmt.Errorf("разбор JSON спецификации: %w", err)
		}
		return &spec, nil
	}
	return nil, fmt.Errorf("gemini недоступен после %d попыток: %v", maxRetries, lastErr)
}

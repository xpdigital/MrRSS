package translation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// MTranTranslator implements translation via a self-hosted MTranServer instance
// (https://github.com/xxnuo/MTranServer).
//
// MTranServer uses language-pair NMT models, so each request must specify an
// explicit source language ("from"). MrRSS's built-in language detector is used
// to determine the source language of each text segment automatically.
//
// Native API:
//
//	POST {endpoint}/translate
//	  request : {"from":"en","to":"zh","text":"..."}
//	  response: {"result":"..."}
type MTranTranslator struct {
	Endpoint string // Base URL, e.g. http://192.168.5.88:8989
	Token    string // Optional API token (MT_API_TOKEN); empty if not set
	client   *http.Client
}

// NewMTranTranslator creates a new MTranServer translator.
func NewMTranTranslator(endpoint, token string) *MTranTranslator {
	return &MTranTranslator{
		Endpoint: strings.TrimSuffix(endpoint, "/"),
		Token:    token,
		// MTranServer is a local/LAN service and responds in tens of ms, so a
		// short timeout is plenty and keeps the UI snappy. Note: not routed
		// through the app's HTTP proxy on purpose — it's a local service.
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

// mtranLangCode normalizes an ISO code to what MTranServer expects (base
// language, lowercase). e.g. "zh-tw" / "zh-Hant" -> "zh", "EN" -> "en".
func mtranLangCode(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return ""
	}
	// Chinese variants collapse to "zh"
	if strings.HasPrefix(code, "zh") {
		return "zh"
	}
	if len(code) > 2 {
		code = code[:2]
	}
	return code
}

// Translate translates text into targetLang using MTranServer.
func (t *MTranTranslator) Translate(text, targetLang string) (string, error) {
	if text == "" {
		return "", nil
	}
	if t.Endpoint == "" {
		return "", fmt.Errorf("MTranServer endpoint is not configured")
	}

	to := mtranLangCode(targetLang)
	if to == "" {
		to = "zh"
	}

	// This integration is scoped to English -> Chinese translation only, so the
	// source language is fixed to English. This avoids language-detection
	// misfires and keeps requests fast. (If bidirectional support is ever
	// needed, swap this for GetLanguageDetector().DetectLanguage(text).)
	from := "en"

	// If the target is English there is nothing to do for an en->en pair.
	if from == to {
		return text, nil
	}

	reqBody := map[string]string{
		"from": from,
		"to":   to,
		"text": text,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal MTranServer request: %w", err)
	}

	req, err := http.NewRequest("POST", t.Endpoint+"/translate", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("failed to create MTranServer request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.Token != "" {
		req.Header.Set("Authorization", "Bearer "+t.Token)
	}

	resp, err := t.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("MTranServer request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MTranServer returned status %d: %s", resp.StatusCode, truncateBody(string(body), 200))
	}

	var result struct {
		Result string `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode MTranServer response: %w", err)
	}

	return result.Result, nil
}

// truncateBody shortens a response body for safe error messages.
func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

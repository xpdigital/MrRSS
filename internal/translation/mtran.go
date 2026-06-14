package translation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
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
	// source language is fixed to English.
	from := "en"

	// If the target is English there is nothing to do for an en->en pair.
	if from == to {
		return text, nil
	}

	// Only translate text that is purely English. If it contains any Chinese
	// (Han) characters it's treated as already-Chinese content (e.g. feeds like
	// 逛逛GitHub) and returned untouched — feeding such text to the en->zh model
	// would mangle it into garbage ("腾讯开源了…" -> "门 腾讯开源纬 特…").
	if containsChinese(text) {
		return text, nil
	}

	// Protect brand names so the model can't translate them (e.g. "Apple" ->
	// "苹果"). Each brand is swapped for an opaque placeholder before
	// translation and restored afterwards.
	protectedText, restore := protectBrands(text)

	reqBody := map[string]string{
		"from": from,
		"to":   to,
		"text": protectedText,
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

	// Restore the original brand names in place of the placeholders.
	return restore(result.Result), nil
}

// protectedBrands lists English brand/product names that should always be kept
// as-is and never translated to Chinese. Matched case-insensitively on whole
// words. Order matters: longer, more specific names come before shorter ones
// (e.g. "Apple Watch" before "Apple") so the more specific match wins.
var protectedBrands = []string{
	"Apple Watch", "Apple TV", "Apple Music", "Apple Vision Pro", "Apple",
	"Google Pixel", "Google", "Microsoft", "Amazon", "Meta", "Tesla",
	"OpenAI", "ChatGPT", "Claude", "Anthropic", "Gemini", "Nvidia", "AMD", "Intel",
	"iPhone", "iPad", "iPadOS", "iOS", "macOS", "watchOS", "tvOS", "visionOS", "Mac", "MacBook", "AirPods", "Siri",
	"Android", "Pixel", "Chrome", "ChromeOS", "Windows", "Surface", "Xbox", "Copilot",
	"GitHub", "GitLab", "Slack", "Notion", "Figma", "Spotify", "Netflix", "YouTube", "TikTok",
	"DeepSeek", "Qwen", "Llama", "Mistral", "Nintendo", "Switch", "PlayStation", "Sony", "Samsung", "Galaxy", "Huawei", "Xiaomi",
}

// brandRegexpCache memoizes the compiled whole-word, case-insensitive regexp
// for each brand so repeated translations don't recompile them.
var (
	brandRegexpCache = map[string]*regexp.Regexp{}
	brandRegexpMu    sync.Mutex
)

// brandWordRegexp returns a cached case-insensitive, whole-word regexp for the
// given brand name.
func brandWordRegexp(brand string) *regexp.Regexp {
	brandRegexpMu.Lock()
	defer brandRegexpMu.Unlock()
	if re, ok := brandRegexpCache[brand]; ok {
		return re
	}
	// (?i) case-insensitive; \b word boundaries keep it whole-word so "Meta"
	// doesn't match "Metaphor". The optional (?:es|s)? suffix matches plural
	// product names so "Apple Watch" also catches "Apple Watches" and "iPad"
	// catches "iPads" — otherwise the plural's "...es"/"...s" tail gets
	// translated on its own ("Apple Watches" -> "Apple 观看"). The matched text
	// (including any plural suffix) is preserved verbatim on restore.
	// Brand names are ASCII and only pure-English text reaches this point, so
	// ASCII word boundaries are sufficient.
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(brand) + `(?:es|s)?\b`)
	brandRegexpCache[brand] = re
	return re
}

// brandPlaceholder builds the opaque token used to stand in for a brand during
// translation. "BRND" is a distinctive core unlikely to appear in real text;
// the @@ wrappers help set it apart. Note the NMT model sometimes alters the
// wrapper (e.g. dropping an @), which is why restoreBrandsRegexp is tolerant.
func brandPlaceholder(i int) string {
	return fmt.Sprintf("@@BRND%d@@", i)
}

// restoreBrandsRegexp matches a (possibly model-mangled) brand placeholder and
// captures its index. It tolerates a varying number of @ wrappers, spaces the
// model may insert anywhere inside the token, and case changes — so
// "@@BRND1@@", "@BRND1@@", "@@ BRND 1 @@", "brnd1" all match and restore.
// Only single spaces are consumed (not arbitrary whitespace) to avoid eating
// meaningful gaps between words.
var restoreBrandsRegexp = regexp.MustCompile(`(?i)@*[ ]?BRND[ ]*(\d+)[ ]?@*`)

// protectBrands replaces known brand names in text with placeholders and
// returns the protected text plus a restore function that swaps the original
// brand names back in. Matching is case-insensitive and whole-word.
func protectBrands(text string) (string, func(string) string) {
	originalByIdx := map[int]string{}

	protected := text
	idx := 0
	for _, brand := range protectedBrands {
		re := brandWordRegexp(brand)
		loc := re.FindStringIndex(protected)
		if loc == nil {
			continue
		}
		// Preserve the original (first-matched) spelling, then replace every
		// occurrence of this brand with the same placeholder.
		original := protected[loc[0]:loc[1]]
		protected = re.ReplaceAllString(protected, brandPlaceholder(idx))
		originalByIdx[idx] = original
		idx++
	}

	restore := func(s string) string {
		s = restoreBrandsRegexp.ReplaceAllStringFunc(s, func(m string) string {
			sub := restoreBrandsRegexp.FindStringSubmatch(m)
			if sub == nil {
				return m
			}
			i, err := strconv.Atoi(sub[1])
			if err != nil {
				return m
			}
			if orig, ok := originalByIdx[i]; ok {
				return orig
			}
			// Unknown index (shouldn't happen): drop the stray placeholder
			// rather than leaking it into the output.
			return ""
		})

		// The NMT model occasionally emits a placeholder twice in a row, which
		// restores to a duplicated brand ("SamsungSamsung"). Collapse any
		// brand immediately repeated (with or without a space) back to one.
		for _, orig := range originalByIdx {
			for strings.Contains(s, orig+orig) {
				s = strings.ReplaceAll(s, orig+orig, orig)
			}
			s = strings.ReplaceAll(s, orig+" "+orig, orig)
		}
		return s
	}
	return protected, restore
}

// containsChinese reports whether text contains any Chinese (Han) character.
// The MTranServer integration only translates purely-English text, so any Han
// character means the content is already Chinese and must be left untouched.
func containsChinese(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// truncateBody shortens a response body for safe error messages.
func truncateBody(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

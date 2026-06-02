package service

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

var ErrSensitiveResponseBlocked = errors.New("sensitive words detected")

type SensitiveFilterMatch struct {
	RuleID   string `json:"rule_id"`
	RuleName string `json:"rule_name"`
	Action   string `json:"action"`
	Keyword  string `json:"keyword"`
}

type SensitiveFilterResult struct {
	Blocked bool
	Mutated bool
	Matches []SensitiveFilterMatch
}

type SensitiveStreamDataFilterResult struct {
	Blocked bool
	Mutated bool
	Held    bool
	Matches []SensitiveFilterMatch
	Data    []string
}

type compiledSensitiveRule struct {
	setting.SensitiveRule
	order    int
	keywords []compiledSensitiveKeyword
}

type compiledSensitiveKeyword struct {
	origin string
	lower  string
	runes  []rune
}

type textRangeMatch struct {
	start int
	end   int
	rule  compiledSensitiveRule
	word  compiledSensitiveKeyword
}

type sensitiveTextFilter struct {
	blockRules []compiledSensitiveRule
	maskRules  []compiledSensitiveRule
}

const sensitiveStreamBlockTailContextKey = "sensitive_response_stream_block_tail"
const sensitiveStreamDelayBufferContextKey = "sensitive_response_stream_delay_buffer"

type sensitiveStreamDelayBuffer struct {
	lookbehind int
	queue      []string
	textRunes  int
}

func ApplySensitiveFilterToRequestBody(c *gin.Context, relayFormat types.RelayFormat) (*SensitiveFilterResult, error) {
	result := &SensitiveFilterResult{}
	if !shouldRunSensitiveFilter(c) || c == nil || c.Request == nil || !setting.ShouldCheckPromptSensitive() {
		return result, nil
	}
	filter := newSensitiveTextFilter(setting.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeRequest))
	if filter.empty() {
		return result, nil
	}
	if !isJSONContentType(c.Request.Header.Get("Content-Type")) {
		return result, nil
	}

	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, err
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return result, nil
	}

	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	blockScan := &requestTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processRelayTextFields(payload, relayFormat, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		return result, nil
	}

	maskScan := &requestTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processRelayTextFields(payload, relayFormat, maskScan)
	if !maskScan.mutated {
		return result, nil
	}

	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	newStorage, err := common.CreateBodyStorage(rewritten)
	if err != nil {
		return nil, err
	}
	_ = storage.Close()
	c.Set(common.KeyBodyStorage, newStorage)
	if _, err := newStorage.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	c.Request.Body = io.NopCloser(newStorage)
	c.Request.ContentLength = int64(len(rewritten))

	result.Mutated = true
	result.Matches = maskScan.matches
	return result, nil
}

func ApplySensitiveFilterToResponseBody(c *gin.Context, contentType string, body []byte) (*SensitiveFilterResult, []byte, error) {
	result := &SensitiveFilterResult{}
	if !shouldRunSensitiveFilter(c) {
		return result, body, nil
	}
	filter := newSensitiveTextFilter(setting.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse))
	if filter.empty() {
		return result, body, nil
	}
	if !isJSONContentType(contentType) {
		return result, body, nil
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return result, body, nil
	}

	var payload any
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, nil, err
	}

	blockScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processResponseTextFields(payload, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		return result, body, nil
	}

	maskScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processResponseTextFields(payload, maskScan)
	if !maskScan.mutated {
		return result, body, nil
	}

	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	result.Mutated = true
	result.Matches = maskScan.matches
	return result, rewritten, nil
}

func ApplySensitiveFilterToStreamData(c *gin.Context, data string) (*SensitiveFilterResult, string, error) {
	result := &SensitiveFilterResult{}
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) || !shouldRunSensitiveFilter(c) {
		return result, data, nil
	}
	filter := newSensitiveTextFilter(setting.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse))
	if filter.empty() {
		return result, data, nil
	}

	var payload any
	if err := common.UnmarshalJsonStr(trimmed, &payload); err != nil {
		return nil, "", err
	}

	blockScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionBlock}
	processResponseTextFields(payload, blockScan)
	if len(blockScan.matches) > 0 {
		result.Blocked = true
		result.Matches = blockScan.matches
		return result, data, nil
	}
	if matches := filter.streamBlockMatches(c, payload); len(matches) > 0 {
		result.Blocked = true
		result.Matches = matches
		return result, data, nil
	}

	maskScan := &responseTextProcessor{filter: filter, mode: setting.SensitiveRuleActionMask}
	processResponseTextFields(payload, maskScan)
	if !maskScan.mutated {
		return result, data, nil
	}

	rewritten, err := common.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	result.Mutated = true
	result.Matches = maskScan.matches
	return result, string(rewritten), nil
}

func ApplySensitiveFilterToStreamDataForSend(c *gin.Context, data string) (*SensitiveStreamDataFilterResult, error) {
	if strings.TrimSpace(data) == "[DONE]" {
		return &SensitiveStreamDataFilterResult{Data: []string{data}}, nil
	}
	result, filtered, err := ApplySensitiveFilterToStreamData(c, data)
	if err != nil {
		return nil, err
	}
	streamResult := &SensitiveStreamDataFilterResult{
		Blocked: result.Blocked,
		Mutated: result.Mutated,
		Matches: result.Matches,
	}
	if result.Blocked {
		return streamResult, nil
	}
	readyData, held, err := bufferSensitiveStreamDataForSend(c, filtered)
	if err != nil {
		return nil, err
	}
	streamResult.Data = readyData
	streamResult.Held = held
	return streamResult, nil
}

func FlushSensitiveStreamDataForSend(c *gin.Context) []string {
	buffer := getSensitiveStreamDelayBuffer(c)
	if buffer == nil {
		return nil
	}
	return buffer.flush()
}

func NewSensitiveFilterAPIError(c *gin.Context) *types.NewAPIError {
	apiErr := types.NewError(ErrSensitiveResponseBlocked, types.ErrorCodeSensitiveWordsDetected, types.ErrOptionWithStatusCode(400), types.ErrOptionWithSkipRetry())
	if c != nil {
		if requestId := c.GetString(common.RequestIdKey); requestId != "" {
			apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), requestId))
		}
	}
	return apiErr
}

func SensitiveFilterOpenAIErrorBody(c *gin.Context) []byte {
	body, err := common.Marshal(map[string]any{
		"error": NewSensitiveFilterAPIError(c).ToOpenAIError(),
	})
	if err != nil {
		return []byte(`{"error":{"message":"sensitive words detected","type":"new_api_error","param":"","code":"sensitive_words_detected"}}`)
	}
	return body
}

func shouldRunSensitiveFilter(c *gin.Context) bool {
	if c == nil || !setting.CheckSensitiveEnabled {
		return false
	}
	channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	return setting.ShouldApplySensitiveRulesToChannel(channelId)
}

func FormatSensitiveFilterMatches(matches []SensitiveFilterMatch) string {
	if len(matches) == 0 {
		return ""
	}
	parts := make([]string, 0, len(matches))
	for _, match := range matches {
		name := strings.TrimSpace(match.RuleName)
		if name == "" {
			name = match.RuleID
		}
		parts = append(parts, fmt.Sprintf("%s:%s", match.Action, name))
	}
	return strings.Join(parts, ", ")
}

func isJSONContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	return mediaType == "" || mediaType == "application/json" || strings.HasSuffix(mediaType, "+json")
}

func newSensitiveTextFilter(rules []setting.SensitiveRule) *sensitiveTextFilter {
	filter := &sensitiveTextFilter{}
	for idx, rule := range setting.NormalizeSensitiveRules(rules) {
		if !rule.Enabled {
			continue
		}
		compiled := compiledSensitiveRule{
			SensitiveRule: rule,
			order:         idx,
			keywords:      make([]compiledSensitiveKeyword, 0, len(rule.Keywords)),
		}
		for _, keyword := range rule.Keywords {
			lower := strings.ToLower(keyword)
			compiled.keywords = append(compiled.keywords, compiledSensitiveKeyword{
				origin: keyword,
				lower:  lower,
				runes:  []rune(lower),
			})
		}
		if len(compiled.keywords) == 0 {
			continue
		}
		switch rule.Action {
		case setting.SensitiveRuleActionMask:
			filter.maskRules = append(filter.maskRules, compiled)
		default:
			filter.blockRules = append(filter.blockRules, compiled)
		}
	}
	return filter
}

func (f *sensitiveTextFilter) empty() bool {
	return f == nil || (len(f.blockRules) == 0 && len(f.maskRules) == 0)
}

func (f *sensitiveTextFilter) maxBlockKeywordRunes() int {
	if f == nil {
		return 0
	}
	maxLength := 0
	for _, rule := range f.blockRules {
		for _, keyword := range rule.keywords {
			if length := len(keyword.runes); length > maxLength {
				maxLength = length
			}
		}
	}
	return maxLength
}

func (f *sensitiveTextFilter) blockMatches(text string) []SensitiveFilterMatch {
	if f == nil || text == "" {
		return nil
	}
	lower := strings.ToLower(text)
	var matches []SensitiveFilterMatch
	for _, rule := range f.blockRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(lower, keyword.lower) {
				matches = append(matches, rule.toMatch(keyword))
				break
			}
		}
	}
	return matches
}

func (f *sensitiveTextFilter) streamBlockMatches(c *gin.Context, value any) []SensitiveFilterMatch {
	maxKeywordLength := f.maxBlockKeywordRunes()
	if c == nil || maxKeywordLength <= 1 {
		return nil
	}
	texts := collectResponseTextFields(value)
	if len(texts) == 0 {
		return nil
	}
	currentText := strings.Join(texts, "")
	candidate := c.GetString(sensitiveStreamBlockTailContextKey) + currentText
	if matches := f.blockMatches(candidate); len(matches) > 0 {
		return matches
	}
	c.Set(sensitiveStreamBlockTailContextKey, lastRunes(candidate, maxKeywordLength-1))
	return nil
}

func (f *sensitiveTextFilter) maskText(text string) (string, []SensitiveFilterMatch, bool) {
	if f == nil || text == "" {
		return text, nil, false
	}
	ranges := f.maskRanges(text)
	if len(ranges) == 0 {
		return text, nil, false
	}
	sort.SliceStable(ranges, func(i, j int) bool {
		if ranges[i].start != ranges[j].start {
			return ranges[i].start < ranges[j].start
		}
		if ranges[i].end != ranges[j].end {
			return ranges[i].end > ranges[j].end
		}
		return ranges[i].rule.order < ranges[j].rule.order
	})

	selected := make([]textRangeMatch, 0, len(ranges))
	lastEnd := -1
	for _, item := range ranges {
		if item.start < lastEnd {
			continue
		}
		selected = append(selected, item)
		lastEnd = item.end
	}

	source := []rune(text)
	var builder strings.Builder
	matches := make([]SensitiveFilterMatch, 0, len(selected))
	cursor := 0
	for _, item := range selected {
		builder.WriteString(string(source[cursor:item.start]))
		builder.WriteString(item.rule.Replacement)
		cursor = item.end
		matches = append(matches, item.rule.toMatch(item.word))
	}
	builder.WriteString(string(source[cursor:]))
	return builder.String(), matches, true
}

func (f *sensitiveTextFilter) maskRanges(text string) []textRangeMatch {
	lowerRunes := []rune(strings.ToLower(text))
	var ranges []textRangeMatch
	for _, rule := range f.maskRules {
		for _, keyword := range rule.keywords {
			start := 0
			for start <= len(lowerRunes)-len(keyword.runes) {
				idx := indexRunes(lowerRunes[start:], keyword.runes)
				if idx < 0 {
					break
				}
				absolute := start + idx
				ranges = append(ranges, textRangeMatch{
					start: absolute,
					end:   absolute + len(keyword.runes),
					rule:  rule,
					word:  keyword,
				})
				start = absolute + len(keyword.runes)
			}
		}
	}
	return ranges
}

func indexRunes(text []rune, pattern []rune) int {
	if len(pattern) == 0 || len(text) < len(pattern) {
		return -1
	}
	for i := 0; i <= len(text)-len(pattern); i++ {
		matched := true
		for j := range pattern {
			if text[i+j] != pattern[j] {
				matched = false
				break
			}
		}
		if matched {
			return i
		}
	}
	return -1
}

func (r compiledSensitiveRule) toMatch(keyword compiledSensitiveKeyword) SensitiveFilterMatch {
	return SensitiveFilterMatch{
		RuleID:   r.ID,
		RuleName: r.Name,
		Action:   r.Action,
		Keyword:  keyword.origin,
	}
}

type requestTextProcessor struct {
	filter  *sensitiveTextFilter
	mode    string
	mutated bool
	matches []SensitiveFilterMatch
}

type responseTextProcessor struct {
	filter  *sensitiveTextFilter
	mode    string
	mutated bool
	matches []SensitiveFilterMatch
}

func (p *responseTextProcessor) process(text string) (string, bool) {
	switch p.mode {
	case setting.SensitiveRuleActionBlock:
		matches := p.filter.blockMatches(text)
		p.matches = append(p.matches, matches...)
		return text, false
	case setting.SensitiveRuleActionMask:
		updated, matches, changed := p.filter.maskText(text)
		if changed {
			p.mutated = true
			p.matches = append(p.matches, matches...)
		}
		return updated, changed
	default:
		return text, false
	}
}

func (p *requestTextProcessor) process(text string) (string, bool) {
	switch p.mode {
	case setting.SensitiveRuleActionBlock:
		matches := p.filter.blockMatches(text)
		p.matches = append(p.matches, matches...)
		return text, false
	case setting.SensitiveRuleActionMask:
		updated, matches, changed := p.filter.maskText(text)
		if changed {
			p.mutated = true
			p.matches = append(p.matches, matches...)
		}
		return updated, changed
	default:
		return text, false
	}
}

func processResponseTextFields(value any, processor *responseTextProcessor) {
	processResponseValue(value, "", processor)
}

func collectResponseTextFields(value any) []string {
	var texts []string
	collectResponseTextValue(value, "", &texts)
	return texts
}

func collectResponseTextValue(value any, key string, texts *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			if shouldSkipResponseField(childKey, childValue) {
				continue
			}
			if text, ok := childValue.(string); ok {
				*texts = append(*texts, text)
				continue
			}
			collectResponseTextValue(childValue, childKey, texts)
		}
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok {
				if shouldSkipResponseField(key, text) {
					continue
				}
				*texts = append(*texts, text)
				continue
			}
			collectResponseTextValue(item, key, texts)
		}
	}
}

func processResponseValue(value any, key string, processor *responseTextProcessor) {
	if processor == nil {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for childKey, childValue := range typed {
			if shouldSkipResponseField(childKey, childValue) {
				continue
			}
			if text, ok := childValue.(string); ok {
				updated, changed := processor.process(text)
				if changed {
					typed[childKey] = updated
				}
				continue
			}
			processResponseValue(childValue, childKey, processor)
		}
	case []any:
		for idx, item := range typed {
			if text, ok := item.(string); ok {
				if shouldSkipResponseField(key, text) {
					continue
				}
				updated, changed := processor.process(text)
				if changed {
					typed[idx] = updated
				}
				continue
			}
			processResponseValue(item, key, processor)
		}
	}
}

func shouldSkipResponseField(key string, value any) bool {
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" {
		return false
	}
	switch normalizedKey {
	case "id", "object", "model", "created", "created_at", "updated_at", "system_fingerprint",
		"metadata", "usage", "prompt_tokens", "completion_tokens", "total_tokens", "input_tokens",
		"output_tokens", "cached_tokens", "finish_reason", "index", "role", "type", "status",
		"name", "function", "tool_calls", "tools", "tool_choice":
		return true
	}
	if strings.Contains(normalizedKey, "url") ||
		strings.Contains(normalizedKey, "base64") ||
		strings.Contains(normalizedKey, "b64") ||
		strings.Contains(normalizedKey, "image") ||
		strings.Contains(normalizedKey, "audio") ||
		strings.Contains(normalizedKey, "file") ||
		strings.Contains(normalizedKey, "mime") ||
		strings.Contains(normalizedKey, "schema") {
		return true
	}
	if str, ok := value.(string); ok && looksLikeEncodedOrURL(str) {
		return true
	}
	return false
}

func looksLikeEncodedOrURL(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "http://") ||
		strings.HasPrefix(lower, "https://") ||
		strings.HasPrefix(lower, "data:") {
		return true
	}
	if len(text) < 128 {
		return false
	}
	base64Chars := 0
	for _, r := range text {
		switch {
		case r >= 'a' && r <= 'z':
			base64Chars++
		case r >= 'A' && r <= 'Z':
			base64Chars++
		case r >= '0' && r <= '9':
			base64Chars++
		case r == '+' || r == '/' || r == '=' || r == '-' || r == '_':
			base64Chars++
		}
	}
	return float64(base64Chars)/float64(len([]rune(text))) > 0.95
}

func lastRunes(text string, limit int) string {
	if limit <= 0 || text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[len(runes)-limit:])
}

func bufferSensitiveStreamDataForSend(c *gin.Context, data string) ([]string, bool, error) {
	buffer := getOrCreateSensitiveStreamDelayBuffer(c)
	if buffer == nil {
		return []string{data}, false, nil
	}
	readyData, err := buffer.push(data)
	if err != nil {
		return nil, false, err
	}
	return readyData, len(readyData) == 0, nil
}

func getOrCreateSensitiveStreamDelayBuffer(c *gin.Context) *sensitiveStreamDelayBuffer {
	if c == nil {
		return nil
	}
	if buffer := getSensitiveStreamDelayBuffer(c); buffer != nil {
		return buffer
	}
	if !shouldRunSensitiveFilter(c) {
		return nil
	}
	filter := newSensitiveTextFilter(setting.GetEffectiveSensitiveRulesByScope(setting.SensitiveRuleScopeResponse))
	if filter.empty() {
		return nil
	}
	maxKeywordLength := filter.maxBlockKeywordRunes()
	if maxKeywordLength <= 1 {
		return nil
	}
	buffer := &sensitiveStreamDelayBuffer{lookbehind: maxKeywordLength - 1}
	c.Set(sensitiveStreamDelayBufferContextKey, buffer)
	return buffer
}

func getSensitiveStreamDelayBuffer(c *gin.Context) *sensitiveStreamDelayBuffer {
	if c == nil {
		return nil
	}
	value, ok := c.Get(sensitiveStreamDelayBufferContextKey)
	if !ok {
		return nil
	}
	buffer, _ := value.(*sensitiveStreamDelayBuffer)
	return buffer
}

func (b *sensitiveStreamDelayBuffer) push(data string) ([]string, error) {
	if b == nil || b.lookbehind <= 0 {
		return []string{data}, nil
	}
	textRunes, err := streamTextRuneCount(data)
	if err != nil {
		return nil, err
	}
	b.queue = append(b.queue, data)
	b.textRunes += textRunes
	var ready []string
	for len(b.queue) > 1 && b.textRunes > b.lookbehind {
		first := b.queue[0]
		firstRunes, err := streamTextRuneCount(first)
		if err != nil {
			return nil, err
		}
		if b.textRunes-firstRunes < b.lookbehind {
			break
		}
		ready = append(ready, first)
		b.queue = b.queue[1:]
		b.textRunes -= firstRunes
	}
	return ready, nil
}

func (b *sensitiveStreamDelayBuffer) flush() []string {
	if b == nil || len(b.queue) == 0 {
		return nil
	}
	ready := b.queue
	b.queue = nil
	b.textRunes = 0
	return ready
}

func streamTextRuneCount(data string) (int, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" || (!strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[")) {
		return 0, nil
	}
	var payload any
	if err := common.UnmarshalJsonStr(trimmed, &payload); err != nil {
		return 0, err
	}
	text := strings.Join(collectResponseTextFields(payload), "")
	return len([]rune(text)), nil
}

func processRelayTextFields(payload any, relayFormat types.RelayFormat, processor *requestTextProcessor) {
	obj, ok := payload.(map[string]any)
	if !ok || processor == nil {
		return
	}
	switch relayFormat {
	case types.RelayFormatClaude:
		processClaudePayload(obj, processor)
	case types.RelayFormatGemini:
		processGeminiPayload(obj, processor)
	case types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		processResponsesPayload(obj, processor)
	case types.RelayFormatOpenAIImage:
		processStringKey(obj, "prompt", processor)
	case types.RelayFormatOpenAIAudio:
		processStringKey(obj, "input", processor)
		processStringKey(obj, "instructions", processor)
	case types.RelayFormatEmbedding:
		processStringOrStringArrayKey(obj, "input", processor)
	case types.RelayFormatRerank:
		processStringKey(obj, "query", processor)
		processRerankDocuments(obj["documents"], processor)
	default:
		processOpenAIPayload(obj, processor)
	}
}

func processOpenAIPayload(obj map[string]any, processor *requestTextProcessor) {
	processStringOrStringArrayKey(obj, "prompt", processor)
	processStringOrStringArrayKey(obj, "input", processor)
	processStringOrStringArrayKey(obj, "prefix", processor)
	processStringOrStringArrayKey(obj, "suffix", processor)
	processStringKey(obj, "instruction", processor)
	processOpenAIMessages(obj["messages"], "text", processor)
}

func processResponsesPayload(obj map[string]any, processor *requestTextProcessor) {
	processStringKey(obj, "instructions", processor)
	processStringKey(obj, "prompt", processor)
	processStringOrStringArrayKey(obj, "input", processor)
	processResponsesInput(obj["input"], processor)
}

func processClaudePayload(obj map[string]any, processor *requestTextProcessor) {
	processClaudeContent(obj, "system", processor)
	processOpenAIMessages(obj["messages"], "text", processor)
}

func processGeminiPayload(obj map[string]any, processor *requestTextProcessor) {
	processGeminiContent(obj["systemInstruction"], processor)
	processGeminiContents(obj["contents"], processor)
	processGeminiRequests(obj["requests"], processor)
}

func processOpenAIMessages(value any, textType string, processor *requestTextProcessor) {
	messages, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range messages {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		processTypedContent(message, "content", textType, processor)
	}
}

func processClaudeContent(obj map[string]any, key string, processor *requestTextProcessor) {
	processTypedContent(obj, key, "text", processor)
}

func processResponsesInput(value any, processor *requestTextProcessor) {
	switch input := value.(type) {
	case string:
		updated, changed := processor.process(input)
		if changed {
			// Caller holds the map for direct key updates; this branch is handled
			// by processStringKey where possible.
			_ = updated
		}
	case []any:
		for _, item := range input {
			inputItem, ok := item.(map[string]any)
			if !ok {
				continue
			}
			processTypedContent(inputItem, "content", "input_text", processor)
		}
	}
}

func processTypedContent(obj map[string]any, key string, textType string, processor *requestTextProcessor) {
	switch content := obj[key].(type) {
	case string:
		updated, changed := processor.process(content)
		if changed {
			obj[key] = updated
		}
	case []any:
		for _, partAny := range content {
			part, ok := partAny.(map[string]any)
			if !ok {
				continue
			}
			if partType, _ := part["type"].(string); partType == textType {
				processStringKey(part, "text", processor)
			}
		}
	}
}

func processGeminiRequests(value any, processor *requestTextProcessor) {
	requests, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range requests {
		if request, ok := item.(map[string]any); ok {
			processGeminiPayload(request, processor)
		}
	}
}

func processGeminiContents(value any, processor *requestTextProcessor) {
	contents, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range contents {
		processGeminiContent(item, processor)
	}
}

func processGeminiContent(value any, processor *requestTextProcessor) {
	content, ok := value.(map[string]any)
	if !ok {
		return
	}
	parts, ok := content["parts"].([]any)
	if !ok {
		return
	}
	for _, partAny := range parts {
		part, ok := partAny.(map[string]any)
		if !ok {
			continue
		}
		processStringKey(part, "text", processor)
	}
}

func processRerankDocuments(value any, processor *requestTextProcessor) {
	documents, ok := value.([]any)
	if !ok {
		return
	}
	for index, item := range documents {
		switch document := item.(type) {
		case string:
			updated, changed := processor.process(document)
			if changed {
				documents[index] = updated
			}
		case map[string]any:
			processStringKey(document, "text", processor)
		}
	}
}

func processStringOrStringArrayKey(obj map[string]any, key string, processor *requestTextProcessor) {
	switch value := obj[key].(type) {
	case string:
		updated, changed := processor.process(value)
		if changed {
			obj[key] = updated
		}
	case []any:
		for idx, item := range value {
			if str, ok := item.(string); ok {
				updated, changed := processor.process(str)
				if changed {
					value[idx] = updated
				}
			}
		}
	}
}

func processStringKey(obj map[string]any, key string, processor *requestTextProcessor) {
	value, ok := obj[key].(string)
	if !ok {
		return
	}
	updated, changed := processor.process(value)
	if changed {
		obj[key] = updated
	}
}

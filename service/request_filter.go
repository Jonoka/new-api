package service

import (
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

func ApplySensitiveFilterToRequestBody(c *gin.Context, relayFormat types.RelayFormat) (*SensitiveFilterResult, error) {
	result := &SensitiveFilterResult{}
	if c == nil || c.Request == nil || !setting.ShouldCheckPromptSensitive() {
		return result, nil
	}
	channelId := common.GetContextKeyInt(c, constant.ContextKeyChannelId)
	if !setting.ShouldApplySensitiveRulesToChannel(channelId) {
		return result, nil
	}
	filter := newSensitiveTextFilter(setting.GetEffectiveSensitiveRules())
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

package mempalace

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
)

type MemoryItem struct {
	Text   string
	Score  float64
	Wing   string
	Room   string
	ID     string
	Source string
}

var callToolFn = CallTool
var timeNow = time.Now

// SearchContextIfEnabled searches MemPalace for a behavioral instruction matching
// the query, filtered to the configured wing/room. Returns "" on any failure so
// callers can fall back to roles without interrupting the user.
func SearchContextIfEnabled(ctx context.Context, query string) (string, error) {
	if !viper.GetBool("mempalace.context.enabled") {
		return "", nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", nil
	}
	wing := strings.TrimSpace(viper.GetString("mempalace.context.wing"))
	room := strings.TrimSpace(viper.GetString("mempalace.context.room"))
	maxResults := viper.GetInt("mempalace.context.max_results")
	if maxResults <= 0 {
		maxResults = 1
	}
	minScore := viper.GetFloat64("mempalace.context.min_score")

	args := map[string]interface{}{"query": query}
	if maxResults > 0 {
		args["max_results"] = maxResults
	}
	if minScore > 0 {
		args["min_score"] = minScore
	}
	if wing != "" {
		args["wing"] = wing
	}
	if room != "" {
		args["room"] = room
	}

	raw, err := callToolFn(ctx, "mempalace_search", args)
	if err != nil {
		logEvent("context_search_failed", map[string]interface{}{"error": err.Error()})
		return "", nil
	}
	items := parseMemoryItems(raw)
	if minScore > 0 {
		filtered := items[:0]
		for _, item := range items {
			if item.Score == 0 || item.Score >= minScore {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if len(items) == 0 {
		return "", nil
	}
	return strings.TrimSpace(items[0].Text), nil
}

func InjectIfEnabled(ctx context.Context, query string) (string, error) {
	if !viper.GetBool("mempalace.inject.enabled") {
		return "", nil
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return "", nil
	}
	maxResults := viper.GetInt("mempalace.inject.max_results")
	minScore := viper.GetFloat64("mempalace.inject.min_score")
	items, raw, err := searchMemories(ctx, query, maxResults, minScore)
	if err != nil {
		return "", err
	}
	return BuildMemoryContext(items, raw), nil
}

func searchMemories(ctx context.Context, query string, maxResults int, minScore float64) ([]MemoryItem, json.RawMessage, error) {
	args := map[string]interface{}{"query": query}
	if maxResults > 0 {
		args["max_results"] = maxResults
	}
	if minScore > 0 {
		args["min_score"] = minScore
	}
	raw, err := callToolFn(ctx, "mempalace_search", args)
	if err != nil && (maxResults > 0 || minScore > 0) {
		raw, err = callToolFn(ctx, "mempalace_search", map[string]interface{}{"query": query})
	}
	if err != nil {
		return nil, nil, err
	}
	items := parseMemoryItems(raw)
	if minScore > 0 && len(items) > 0 {
		filtered := items[:0]
		for _, item := range items {
			if item.Score == 0 || item.Score >= minScore {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	if maxResults > 0 && len(items) > maxResults {
		items = items[:maxResults]
	}
	return items, raw, nil
}

func DiaryWriteIfEnabled(ctx context.Context, query, response string) error {
	if !viper.GetBool("mempalace.diary.enabled") {
		return nil
	}
	query = strings.TrimSpace(query)
	response = strings.TrimSpace(response)
	if query == "" || response == "" {
		return nil
	}
	_, err := callToolFn(ctx, "mempalace_diary_write", map[string]interface{}{
		"query":    query,
		"response": response,
	})
	return err
}

func PersistAskResponse(ctx context.Context, query, response string) error {
	query = strings.TrimSpace(query)
	response = strings.TrimSpace(response)
	if response == "" {
		return nil
	}
	return persistDrawer(ctx, "ask", response, query)
}

func PersistChatTurn(ctx context.Context, sessionID string, turn int, userText, assistantText string) error {
	userText = strings.TrimSpace(userText)
	assistantText = strings.TrimSpace(assistantText)
	if userText == "" || assistantText == "" {
		return nil
	}
	content := fmt.Sprintf("session: %s\nturn: %d\nuser: %s\nassistant: %s", strings.TrimSpace(sessionID), turn, userText, assistantText)
	return persistDrawer(ctx, "chat", content, userText)
}

func PersistInvestigateResult(ctx context.Context, goal, finalAnswer string) error {
	goal = strings.TrimSpace(goal)
	finalAnswer = strings.TrimSpace(finalAnswer)
	if goal == "" || finalAnswer == "" {
		return nil
	}
	content := fmt.Sprintf("goal: %s\nfinal_answer: %s", goal, finalAnswer)
	return persistDrawer(ctx, "investigate", content, goal)
}

func PersistToolExecution(ctx context.Context, command, outcome string, exitCode int) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}
	content := fmt.Sprintf("command: %s\nexit_code: %d\noutcome: %s", command, exitCode, strings.TrimSpace(outcome))
	return persistDrawer(ctx, "tool", content, command)
}

func PersistRoleDecision(ctx context.Context, inputText, selectedRole, reason string) error {
	inputText = strings.TrimSpace(inputText)
	selectedRole = strings.TrimSpace(selectedRole)
	if selectedRole == "" {
		return nil
	}
	content := fmt.Sprintf("input: %s\nselected_role: %s\nreason: %s", inputText, selectedRole, strings.TrimSpace(reason))
	return persistDrawer(ctx, "roles", content, inputText)
}

func persistDrawer(ctx context.Context, room, content, query string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	room = strings.TrimSpace(room)
	if room == "" {
		room = "misc"
	}
	args := map[string]interface{}{
		"wing":    "gaia",
		"room":    room,
		"content": content,
	}
	if strings.TrimSpace(query) != "" {
		args["query"] = strings.TrimSpace(query)
	}
	start := timeNow()
	_, err := callToolFn(ctx, "mempalace_add_drawer", args)
	latency := timeNow().Sub(start)
	if err != nil {
		logEvent("drawer_write_failed", map[string]interface{}{
			"tool":       "mempalace_add_drawer",
			"wing":       "gaia",
			"room":       room,
			"latency_ms": latency.Milliseconds(),
			"error":      err.Error(),
		})
		return err
	}
	logEvent("drawer_write_ok", map[string]interface{}{
		"tool":       "mempalace_add_drawer",
		"wing":       "gaia",
		"room":       room,
		"latency_ms": latency.Milliseconds(),
	})
	return nil
}

func BuildMemoryContext(items []MemoryItem, raw json.RawMessage) string {
	if len(items) == 0 {
		if len(raw) == 0 {
			return ""
		}
		return "Memory Context (raw):\n" + formatRaw(raw)
	}
	var b strings.Builder
	b.WriteString("Memory Context:\n")
	for _, item := range items {
		line := formatItemLine(item)
		if line != "" {
			b.WriteString("- ")
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func formatItems(items []MemoryItem, raw json.RawMessage) string {
	if len(items) == 0 {
		return formatRaw(raw)
	}
	var b strings.Builder
	for _, item := range items {
		line := formatItemLine(item)
		if line == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func formatItemsWrapped(items []MemoryItem, raw json.RawMessage, innerWidth int) string {
	if len(items) == 0 {
		return formatRaw(raw)
	}
	if innerWidth <= 0 {
		innerWidth = 78
	}
	var b strings.Builder
	for _, item := range items {
		line := formatItemLine(item)
		if line == "" {
			continue
		}
		wrapped := wrapPreservingCodeBlocks(line, innerWidth-2)
		lines := strings.Split(wrapped, "\n")
		for i, ln := range lines {
			if i == 0 {
				b.WriteString("- ")
				b.WriteString(ln)
				b.WriteString("\n")
				continue
			}
			b.WriteString("  ")
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

func formatItemLine(item MemoryItem) string {
	text := strings.TrimSpace(item.Text)
	if text == "" {
		return ""
	}
	meta := []string{}
	if item.Score > 0 {
		meta = append(meta, fmt.Sprintf("score=%.3f", item.Score))
	}
	if item.Wing != "" {
		meta = append(meta, "wing="+item.Wing)
	}
	if item.Room != "" {
		meta = append(meta, "room="+item.Room)
	}
	if item.Source != "" {
		meta = append(meta, "source="+item.Source)
	}
	if item.ID != "" {
		meta = append(meta, "id="+item.ID)
	}
	if len(meta) == 0 {
		return text
	}
	return fmt.Sprintf("(%s) %s", strings.Join(meta, ", "), text)
}

func wrapPreservingCodeBlocks(input string, width int) string {
	if width <= 0 {
		return input
	}
	lines := strings.Split(input, "\n")
	out := make([]string, 0, len(lines))
	inCode := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			out = append(out, wrapWithIndent(line, width, false)...)
			continue
		}
		if inCode {
			out = append(out, wrapWithIndent(line, width, false)...)
			continue
		}
		out = append(out, wrapWithIndent(line, width, true)...)
	}
	return strings.Join(out, "\n")
}

func wrapWithIndent(line string, width int, softWordWrap bool) []string {
	if line == "" {
		return []string{""}
	}
	prefix := leadingWhitespace(line)
	content := strings.TrimLeft(line, " \t")
	prefixWidth := lipgloss.Width(prefix)
	available := width - prefixWidth
	if available < 8 {
		available = 8
	}

	var wrapped []string
	if softWordWrap {
		wrapped = wrapWords(content, available)
	} else {
		wrapped = wrapRunes(content, available)
	}
	for i := range wrapped {
		wrapped[i] = prefix + wrapped[i]
	}
	return wrapped
}

func leadingWhitespace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[:i]
}

func wrapWords(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}
	lines := []string{}
	current := words[0]
	for _, w := range words[1:] {
		candidate := current + " " + w
		if lipgloss.Width(candidate) <= width {
			current = candidate
			continue
		}
		lines = append(lines, current)
		if lipgloss.Width(w) <= width {
			current = w
		} else {
			parts := wrapRunes(w, width)
			lines = append(lines, parts[:len(parts)-1]...)
			current = parts[len(parts)-1]
		}
	}
	lines = append(lines, current)
	return lines
}

func wrapRunes(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}
	r := []rune(text)
	if len(r) == 0 {
		return []string{""}
	}
	lines := []string{}
	for len(r) > 0 {
		end := width
		if end > len(r) {
			end = len(r)
		}
		lines = append(lines, string(r[:end]))
		r = r[end:]
	}
	return lines
}

func parseMemoryItems(raw json.RawMessage) []MemoryItem {
	raw = unwrapToolContentJSON(raw)
	if len(raw) == 0 {
		return nil
	}
	var rootArray []json.RawMessage
	if err := json.Unmarshal(raw, &rootArray); err == nil {
		return parseItemsRawArray(rootArray)
	}

	var rootObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rootObj); err != nil {
		return nil
	}
	for _, key := range []string{"results", "matches", "items", "hits", "memories"} {
		if payload, ok := rootObj[key]; ok {
			var arr []json.RawMessage
			if err := json.Unmarshal(payload, &arr); err == nil {
				return parseItemsRawArray(arr)
			}
		}
	}
	if payload, ok := rootObj["result"]; ok {
		var arr []json.RawMessage
		if err := json.Unmarshal(payload, &arr); err == nil {
			return parseItemsRawArray(arr)
		}
		var single map[string]interface{}
		if err := json.Unmarshal(payload, &single); err == nil {
			return []MemoryItem{parseItemMap(single)}
		}
	}
	return nil
}

func unwrapToolContentJSON(raw json.RawMessage) json.RawMessage {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return raw
	}
	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return raw
	}
	for _, item := range envelope.Content {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		textRaw := json.RawMessage(text)
		var js any
		if err := json.Unmarshal(textRaw, &js); err == nil {
			return textRaw
		}
	}
	return raw
}

func parseItemsRawArray(arr []json.RawMessage) []MemoryItem {
	items := make([]MemoryItem, 0, len(arr))
	for _, entry := range arr {
		var text string
		if err := json.Unmarshal(entry, &text); err == nil {
			items = append(items, MemoryItem{Text: text})
			continue
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(entry, &obj); err == nil {
			items = append(items, parseItemMap(obj))
			continue
		}
		items = append(items, MemoryItem{Text: string(entry)})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Score > items[j].Score
	})
	return items
}

func parseItemMap(m map[string]interface{}) MemoryItem {
	item := MemoryItem{
		Text:   firstString(m, "text", "content", "chunk", "summary", "body", "snippet", "quote"),
		Wing:   firstString(m, "wing"),
		Room:   firstString(m, "room"),
		ID:     firstString(m, "id", "drawer_id"),
		Source: firstString(m, "source", "path"),
	}
	if score, ok := firstFloat(m, "score", "similarity", "relevance"); ok {
		item.Score = score
	}
	if item.Text == "" {
		if raw, err := json.Marshal(m); err == nil {
			item.Text = string(raw)
		}
	}
	return item
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		val, ok := m[key]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case string:
			if strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func firstFloat(m map[string]interface{}, keys ...string) (float64, bool) {
	for _, key := range keys {
		val, ok := m[key]
		if !ok {
			continue
		}
		switch v := val.(type) {
		case float64:
			return v, true
		case int:
			return float64(v), true
		}
	}
	return 0, false
}

func AppendMemory(systemPrompt, memory string) string {
	memory = strings.TrimSpace(memory)
	if memory == "" {
		return systemPrompt
	}
	if strings.TrimSpace(systemPrompt) == "" {
		return memory
	}
	return strings.TrimSpace(systemPrompt) + "\n\n" + memory
}

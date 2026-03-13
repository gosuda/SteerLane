package slack

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	slackapi "github.com/slack-go/slack"
)

const (
	iconCheckMark   = ":white_check_mark:"
	iconQuestion    = ":question:"
	iconRaisingHand = ":raising_hand:"
	iconX           = ":x:"
	iconWastebasket = ":wastebasket:"
)

// TaskCardInput represents the data needed to build a task card.
type TaskCardInput struct {
	Title        string
	Description  string
	Status       string
	ProjectName  string
	DashboardURL string
	Priority     int
}

// ADRCardInput contains data for rendering an ADR summary card.
type ADRCardInput struct {
	Consequences *struct {
		Good    []string
		Bad     []string
		Neutral []string
	}
	Title       string
	Status      string
	CreatedDate string
	Decision    string
	Sequence    int
}

// SessionStatusInput contains data for rendering a session status card.
type SessionStatusInput struct {
	Status      string
	AgentType   string
	TaskTitle   string
	Detail      string
	StartedTime string
	Elapsed     string
}

// BuildTaskCard renders a task as a Slack Block Kit message.
func BuildTaskCard(input TaskCardInput) []slackapi.Block {
	blocks := make([]slackapi.Block, 0, 4)

	// Header block with task title
	headerText := slackapi.NewTextBlockObject(slackapi.PlainTextType, input.Title, false, false)
	header := slackapi.NewHeaderBlock(headerText)
	blocks = append(blocks, header)

	// Section with fields: Status, Priority, Project name
	taskStatus := taskStatusEmoji(input.Status) + " " + titleCase(input.Status)
	priorityLabel := "P" + itoa(input.Priority)

	fields := []*slackapi.TextBlockObject{
		slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Status*\n"+taskStatus, false, false),
	}
	if input.Priority >= 0 {
		fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Priority*\n"+priorityLabel, false, false))
	}
	if projectName := strings.TrimSpace(input.ProjectName); projectName != "" {
		fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Project*\n"+projectName, false, false))
	}
	section := slackapi.NewSectionBlock(nil, fields, nil)
	blocks = append(blocks, section)

	// Section with description (truncated to 300 chars)
	description := input.Description
	if len(description) > 300 {
		description = substring(description, 0, 300) + "..."
	}
	if description != "" {
		descText := slackapi.NewTextBlockObject(slackapi.MarkdownType, description, false, false)
		blocks = append(blocks, slackapi.NewSectionBlock(descText, nil, nil))
	}

	// Optional action button: "View Dashboard"
	if input.DashboardURL != "" {
		buttonText := slackapi.NewTextBlockObject(slackapi.PlainTextType, "View Dashboard", false, false)
		button := slackapi.NewButtonBlockElement("view_dashboard", "", buttonText)
		button.URL = input.DashboardURL

		action := slackapi.NewActionBlock("", button)
		blocks = append(blocks, action)
	}

	return blocks
}

// BuildADRSummaryCard renders an ADR as a Slack Block Kit message.
func BuildADRSummaryCard(input ADRCardInput) []slackapi.Block {
	blocks := make([]slackapi.Block, 0, 3)

	// Header: "ADR-{sequence}: {title}"
	headerText := slackapi.NewTextBlockObject(slackapi.PlainTextType, fmt.Sprintf("ADR-%d: %s", input.Sequence, input.Title), false, false)
	header := slackapi.NewHeaderBlock(headerText)
	blocks = append(blocks, header)

	// Section fields: Status, Created date
	adrStatus := adrStatusEmoji(input.Status) + " " + titleCase(input.Status)

	fields := []*slackapi.TextBlockObject{
		slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Status*\n"+adrStatus, false, false),
		slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Created*\n"+input.CreatedDate, false, false),
	}
	section := slackapi.NewSectionBlock(nil, fields, nil)
	blocks = append(blocks, section)

	// Section with decision text (truncated to 500 chars)
	decision := input.Decision
	if len(decision) > 500 {
		decision = substring(decision, 0, 500) + "..."
	}
	if decision != "" {
		decisionText := slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Decision*\n"+decision, false, false)
		blocks = append(blocks, slackapi.NewSectionBlock(decisionText, nil, nil))
	}

	// Consequences section if present
	if input.Consequences != nil && (len(input.Consequences.Good) > 0 || len(input.Consequences.Bad) > 0 || len(input.Consequences.Neutral) > 0) {
		var consequencesText string
		if len(input.Consequences.Good) > 0 {
			consequencesText += "*Good outcomes:*\n"
			var consequencesTextSb121 strings.Builder
			for _, good := range input.Consequences.Good {
				consequencesTextSb121.WriteString("• " + good + "\n")
			}
			consequencesText += consequencesTextSb121.String()
		}
		if len(input.Consequences.Bad) > 0 {
			consequencesText += "*Bad outcomes:*\n"
			var consequencesTextSb127 strings.Builder
			for _, bad := range input.Consequences.Bad {
				consequencesTextSb127.WriteString("• " + bad + "\n")
			}
			consequencesText += consequencesTextSb127.String()
		}
		if len(input.Consequences.Neutral) > 0 {
			consequencesText += "*Neutral outcomes:*\n"
			var consequencesTextSb133 strings.Builder
			for _, neutral := range input.Consequences.Neutral {
				consequencesTextSb133.WriteString("• " + neutral + "\n")
			}
			consequencesText += consequencesTextSb133.String()
		}
		if consequencesText != "" {
			consequencesBlock := slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Consequences*\n"+consequencesText, false, false)
			blocks = append(blocks, slackapi.NewSectionBlock(consequencesBlock, nil, nil))
		}
	}

	return blocks
}

// BuildSessionStatusCard renders an agent session status.
func BuildSessionStatusCard(input SessionStatusInput) []slackapi.Block {
	blocks := make([]slackapi.Block, 0, 2)

	// Section with fields: Session status, Agent type, Task title
	sessionStatus := sessionStatusEmoji(input.Status) + " " + titleCase(input.Status)

	fields := []*slackapi.TextBlockObject{
		slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Status*\n"+sessionStatus, false, false),
	}
	if agentType := strings.TrimSpace(input.AgentType); agentType != "" {
		fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Agent*\n"+agentType, false, false))
	}
	if taskTitle := strings.TrimSpace(input.TaskTitle); taskTitle != "" {
		fields = append(fields, slackapi.NewTextBlockObject(slackapi.MarkdownType, "*Task*\n"+taskTitle, false, false))
	}
	section := slackapi.NewSectionBlock(nil, fields, nil)
	blocks = append(blocks, section)

	if detail := strings.TrimSpace(input.Detail); detail != "" {
		detailText := slackapi.NewTextBlockObject(slackapi.MarkdownType, detail, false, false)
		blocks = append(blocks, slackapi.NewSectionBlock(detailText, nil, nil))
	}

	// Context block with timing info (started, elapsed)
	timingLines := make([]string, 0, 2)
	if startedTime := strings.TrimSpace(input.StartedTime); startedTime != "" {
		timingLines = append(timingLines, "*Started:* "+startedTime)
	}
	if elapsed := strings.TrimSpace(input.Elapsed); elapsed != "" {
		timingLines = append(timingLines, "*Elapsed:* "+elapsed)
	}
	if len(timingLines) != 0 {
		timingText := slackapi.NewTextBlockObject(slackapi.MarkdownType, strings.Join(timingLines, "\n"), false, false)
		context := slackapi.NewContextBlock("", timingText)
		blocks = append(blocks, context)
	}

	return blocks
}

// EncodeBlocks marshals Slack Block Kit content for messenger transport.
func EncodeBlocks(blocks []slackapi.Block) ([]byte, error) {
	if len(blocks) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(blocks)
	if err != nil {
		return nil, fmt.Errorf("marshal slack blocks: %w", err)
	}

	return data, nil
}

// Helper functions

func taskStatusEmoji(status string) string {
	switch status {
	case "backlog":
		return ":inbox_tray:"
	case "in_progress":
		return ":hammer_and_wrench:"
	case "review":
		return ":eyes:"
	case "done":
		return iconCheckMark
	default:
		return iconQuestion
	}
}

func adrStatusEmoji(status string) string {
	switch status {
	case "draft":
		return ":memo:"
	case "proposed":
		return iconRaisingHand
	case "accepted":
		return iconCheckMark
	case "rejected":
		return iconX
	case "deprecated":
		return iconWastebasket
	default:
		return iconQuestion
	}
}

func sessionStatusEmoji(status string) string {
	switch status {
	case "pending":
		return ":hourglass:"
	case "running":
		return ":zap:"
	case "waiting_hitl":
		return iconRaisingHand
	case "completed":
		return iconCheckMark
	case "failed":
		return iconX
	case "cancelled":
		return ":no_entry_sign:"
	default:
		return iconQuestion
	}
}

// titleCase primitive utility.
func titleCase(status string) string {
	if status == "" {
		return status
	}
	// Replace underscores with spaces and capitalize first letter
	result := make([]byte, 0, len(status)+5)
	prevWasUnderscore := true
	for i := range len(status) {
		ch := status[i]
		if ch == '_' {
			result = append(result, ' ')
			prevWasUnderscore = true
		} else {
			if prevWasUnderscore && ch >= 'a' && ch <= 'z' {
				result = append(result, ch-32)
				prevWasUnderscore = false
			} else {
				result = append(result, ch)
				if ch != '_' {
					prevWasUnderscore = false
				}
			}
		}
	}
	return string(result)
}

func substring(s string, start, end int) string {
	if start < 0 {
		start = 0
	}
	if end > len(s) {
		end = len(s)
	}
	if start >= end {
		return ""
	}
	return s[start:end]
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	var neg bool
	if v < 0 {
		v = -v
		neg = true
	}
	var pos int
	for v > 0 {
		b[pos] = '0' + byte(v%10)
		pos++
		v /= 10
	}
	if neg {
		b[pos] = '-'
		pos++
	}
	// Reverse
	result := b[:pos]
	for idx, j := 0, pos-1; idx < j; idx, j = idx+1, j-1 {
		result[idx], result[j] = result[j], result[idx]
	}
	return string(result)
}

// FormatDate formats a time.Time as "YYYY-MM-DD".
func FormatDate(t time.Time) string {
	return t.Format("2006-01-02")
}

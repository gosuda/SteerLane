package slack

import (
	"testing"
	"time"

	slackapi "github.com/slack-go/slack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildTaskCard(t *testing.T) {
	tests := []struct {
		name    string
		input   TaskCardInput
		wantLen int
	}{
		{
			name: "minimal task card",
			input: TaskCardInput{
				Title:        "Test Task",
				Status:       "in_progress",
				Priority:     1,
				ProjectName:  "Test Project",
				DashboardURL: "",
			},
			wantLen: 2,
		},
		{
			name: "task card with description",
			input: TaskCardInput{
				Title:        "Test Task",
				Description:  "This is a test description for the task",
				Status:       "in_progress",
				Priority:     1,
				ProjectName:  "Test Project",
				DashboardURL: "",
			},
			wantLen: 3,
		},
		{
			name: "task card with dashboard button",
			input: TaskCardInput{
				Title:        "Test Task",
				Status:       "in_progress",
				Priority:     1,
				ProjectName:  "Test Project",
				DashboardURL: "https://example.com/dashboard",
			},
			wantLen: 3,
		},
		{
			name: "task card truncated description",
			input: TaskCardInput{
				Title:        "Test Task",
				Description:  generateLongString(350), // 350 characters to trigger truncation
				Status:       "in_progress",
				Priority:     1,
				ProjectName:  "Test Project",
				DashboardURL: "",
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := BuildTaskCard(tt.input)

			assert.NotEmpty(t, blocks, "BuildTaskCard should return non-empty blocks")
			assert.Len(t, blocks, tt.wantLen, "BuildTaskCard should return correct number of blocks")
			assert.Equal(t, slackapi.MBTHeader, blocks[0].BlockType(), "First block should be header")
		})
	}
}

// Helper function for generating long strings in tests.
func generateLongString(n int) string {
	result := make([]byte, n)
	for i := range n {
		result[i] = byte('a' + (i % 26))
	}
	return string(result)
}

func TestBuildADRSummaryCard(t *testing.T) {
	tests := []struct {
		name    string
		input   ADRCardInput
		wantLen int
	}{
		{
			name: "minimal ADR card",
			input: ADRCardInput{
				Sequence:    1,
				Title:       "Test ADR",
				Status:      "accepted",
				CreatedDate: "2024-01-01",
				Decision:    "Test decision text",
			},
			wantLen: 3,
		},
		{
			name: "ADR card with consequences",
			input: ADRCardInput{
				Sequence:    1,
				Title:       "Test ADR",
				Status:      "accepted",
				CreatedDate: "2024-01-01",
				Decision:    "Test decision text",
				Consequences: &struct {
					Good    []string
					Bad     []string
					Neutral []string
				}{
					Good:    []string{"Good outcome 1", "Good outcome 2"},
					Bad:     []string{"Bad outcome 1"},
					Neutral: []string{"Neutral outcome 1"},
				},
			},
			wantLen: 4,
		},
		{
			name: "ADR card truncated decision",
			input: ADRCardInput{
				Sequence:    1,
				Title:       "Test ADR",
				Status:      "accepted",
				CreatedDate: "2024-01-01",
				Decision:    generateLongString(550), // 550 characters to trigger truncation
			},
			wantLen: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := BuildADRSummaryCard(tt.input)

			assert.NotEmpty(t, blocks, "BuildADRSummaryCard should return non-empty blocks")
			assert.Len(t, blocks, tt.wantLen, "BuildADRSummaryCard should return correct number of blocks")
			assert.Equal(t, slackapi.MBTHeader, blocks[0].BlockType(), "First block should be header")

			// Verify header contains sequence and title
			header, ok := blocks[0].(*slackapi.HeaderBlock)
			require.True(t, ok, "First block should be HeaderBlock")
			assert.Contains(t, header.Text.Text, "ADR-1", "Header should contain ADR sequence")
			assert.Contains(t, header.Text.Text, "Test ADR", "Header should contain ADR title")
		})
	}
}

func TestBuildSessionStatusCard(t *testing.T) {
	tests := []struct {
		name  string
		input SessionStatusInput
	}{
		{
			name: "running session",
			input: SessionStatusInput{
				Status:      "running",
				AgentType:   "claude",
				TaskTitle:   "Test Task",
				StartedTime: "2024-01-01 10:00:00",
				Elapsed:     "5m30s",
			},
		},
		{
			name: "waiting HITL session",
			input: SessionStatusInput{
				Status:      "waiting_hitl",
				AgentType:   "claude",
				TaskTitle:   "Test Task",
				StartedTime: "2024-01-01 10:00:00",
				Elapsed:     "10m15s",
			},
		},
		{
			name: "completed session",
			input: SessionStatusInput{
				Status:      "completed",
				AgentType:   "claude",
				TaskTitle:   "Test Task",
				StartedTime: "2024-01-01 10:00:00",
				Elapsed:     "15m45s",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blocks := BuildSessionStatusCard(tt.input)

			assert.NotEmpty(t, blocks, "BuildSessionStatusCard should return non-empty blocks")
			assert.Len(t, blocks, 2, "BuildSessionStatusCard should return 2 blocks")
			assert.Equal(t, slackapi.MBTSection, blocks[0].BlockType(), "First block should be section")
			assert.Equal(t, slackapi.MBTContext, blocks[1].BlockType(), "Second block should be context")
		})
	}
}

func TestTaskStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"backlog", ":inbox_tray:"},
		{"in_progress", ":hammer_and_wrench:"},
		{"review", ":eyes:"},
		{"done", ":white_check_mark:"},
		{"unknown", ":question:"},
		{"", ":question:"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := taskStatusEmoji(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestADRStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"draft", ":memo:"},
		{"proposed", ":raising_hand:"},
		{"accepted", ":white_check_mark:"},
		{"rejected", ":x:"},
		{"deprecated", ":wastebasket:"},
		{"unknown", ":question:"},
		{"", ":question:"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := adrStatusEmoji(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSessionStatusEmoji(t *testing.T) {
	tests := []struct {
		status string
		want   string
	}{
		{"pending", ":hourglass:"},
		{"running", ":zap:"},
		{"waiting_hitl", ":raising_hand:"},
		{"completed", ":white_check_mark:"},
		{"failed", ":x:"},
		{"cancelled", ":no_entry_sign:"},
		{"unknown", ":question:"},
		{"", ":question:"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			got := sessionStatusEmoji(tt.status)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCapitalizeStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"in_progress", "In Progress"},
		{"backlog", "Backlog"},
		{"review", "Review"},
		{"done", "Done"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := titleCase(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSubstring(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		want  string
		start int
		end   int
	}{
		{
			name:  "normal range",
			s:     "hello world",
			start: 0,
			end:   5,
			want:  "hello",
		},
		{
			name:  "start negative",
			s:     "hello world",
			start: -5,
			end:   5,
			want:  "hello",
		},
		{
			name:  "end beyond length",
			s:     "hello",
			start: 0,
			end:   100,
			want:  "hello",
		},
		{
			name:  "empty string",
			s:     "",
			start: 0,
			end:   5,
			want:  "",
		},
		{
			name:  "start >= end",
			s:     "hello",
			start: 5,
			end:   5,
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substring(tt.s, tt.start, tt.end)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		want  string
		input int
	}{
		{"0", 0},
		{"1", 1},
		{"9", 9},
		{"10", 10},
		{"42", 42},
		{"123", 123},
		{"-1", -1},
		{"-42", -42},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := itoa(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFormatDate(t *testing.T) {
	t.Run("formats correctly", func(t *testing.T) {
		tm := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
		got := FormatDate(tm)
		want := "2024-01-15"
		assert.Equal(t, want, got)
	})
}

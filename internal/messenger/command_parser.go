// Package messenger defines the common interface for messenger platform adapters.
//
// Every messenger platform (Slack, Discord, Telegram) implements the Messenger
// interface. The orchestrator and HITL router depend only on this interface,
// keeping platform-specific details confined to adapter packages.
package messenger

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ParsedCommand holds the result of parsing a bot mention message into
// a task title and optional description.
type ParsedCommand struct {
	// Title is the short summary extracted from the first line/sentence.
	Title string

	// Description contains any additional context after the title.
	// Empty if the message had no extra content.
	Description string
}

// botMentionPattern matches Slack-style <@UXXXXXX> mentions at the start of text.
var botMentionPattern = regexp.MustCompile(`^<@[A-Z0-9]+>\s*`)

// maxTitleLen caps the title at a reasonable length for a kanban card.
const maxTitleLen = 200

// ParseCommand extracts a task title and optional description from a bot mention
// message. It is platform-agnostic but handles Slack-style <@UID> prefixes.
//
// Rules:
//  1. Strip the leading bot mention (e.g. "<@U1234>").
//  2. If the remaining text has a newline, the first line is the title
//     and everything after is the description.
//  3. If the remaining text has no newline but exceeds maxTitleLen, split
//     at the last space before maxTitleLen.
//  4. If the text is empty after stripping the mention, return an error.
//  5. Title is trimmed of whitespace; description is trimmed of leading/trailing blank lines.
func ParseCommand(text string) (ParsedCommand, error) {
	// Step 1: strip bot mention prefix.
	cleaned := botMentionPattern.ReplaceAllString(text, "")
	cleaned = strings.TrimSpace(cleaned)

	if cleaned == "" {
		return ParsedCommand{}, fmt.Errorf("messenger.ParseCommand: empty message after stripping mention: %w", ErrEmptyCommand)
	}

	// Step 2: check for newlines.
	if title, desc, ok := strings.Cut(cleaned, "\n"); ok {
		title = strings.TrimSpace(title)
		desc = strings.TrimSpace(desc)

		if title == "" {
			// First line was blank; use the rest as title.
			return splitLong(desc)
		}

		if len(title) > maxTitleLen {
			title = title[:maxTitleLen]
		}

		return ParsedCommand{
			Title:       title,
			Description: desc,
		}, nil
	}

	// Step 3: no newline — split on length if needed.
	return splitLong(cleaned)
}

// splitLong splits a single-line text into title + description if it
// exceeds maxTitleLen, breaking at the last word boundary.
func splitLong(text string) (ParsedCommand, error) {
	if len(text) <= maxTitleLen {
		return ParsedCommand{Title: text}, nil
	}

	// Find last space before maxTitleLen.
	cutoff := maxTitleLen
	if idx := strings.LastIndexByte(text[:cutoff], ' '); idx > 0 {
		cutoff = idx
	}

	return ParsedCommand{
		Title:       strings.TrimSpace(text[:cutoff]),
		Description: strings.TrimSpace(text[cutoff:]),
	}, nil
}

// ErrEmptyCommand is returned when the command text contains no actionable content.
var ErrEmptyCommand = errors.New("empty command")

package telegram

import "strings"

func BuildLinkingDM(linkURL string) string {
	trimmed := strings.TrimSpace(linkURL)
	if trimmed == "" {
		return "Connect your Telegram identity to SteerLane to create tasks, answer HITL questions, and receive notifications."
	}
	return "Connect your Telegram identity to SteerLane to create tasks, answer HITL questions, and receive notifications:\n" + trimmed
}

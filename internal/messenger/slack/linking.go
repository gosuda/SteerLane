package slack

import "strings"

// BuildLinkingDM returns the Slack DM text used for first-contact account linking.
func BuildLinkingDM(linkURL string) string {
	trimmed := strings.TrimSpace(linkURL)
	if trimmed == "" {
		return "Connect your Slack identity to SteerLane to create tasks, answer HITL questions, and receive notifications."
	}

	return "Connect your Slack identity to SteerLane to create tasks, answer HITL questions, and receive notifications:\n" + trimmed
}

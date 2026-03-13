package telegram

import "testing"

func TestBuildLinkingDM(t *testing.T) {
	t.Parallel()
	if got := BuildLinkingDM(""); got == "" {
		t.Fatal("expected non-empty fallback text")
	}
}

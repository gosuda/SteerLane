package slack

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"
)

// SignatureVerifier validates Slack request signatures.
// Implementations range from full HMAC-SHA256 verification in production
// to no-op stubs for development.
type SignatureVerifier interface {
	// Verify checks the X-Slack-Signature header against the request body.
	// Returns nil if the signature is valid or verification is disabled.
	Verify(header http.Header, body []byte) error
}

// hmacVerifier implements HMAC-SHA256 signature verification per
// https://api.slack.com/authentication/verifying-requests-from-slack
type hmacVerifier struct {
	signingSecret string
	// maxTimestampAge is how far back we accept timestamps (prevents replay).
	maxTimestampAge time.Duration
}

// NewHMACVerifier creates a production signature verifier using the Slack
// signing secret. Requests older than 5 minutes are rejected to prevent replay.
func NewHMACVerifier(signingSecret string) SignatureVerifier {
	return &hmacVerifier{
		signingSecret:   signingSecret,
		maxTimestampAge: 5 * time.Minute,
	}
}

func (v *hmacVerifier) Verify(header http.Header, body []byte) error {
	tsStr := header.Get("X-Slack-Request-Timestamp")
	if tsStr == "" {
		return errors.New("slack verify: missing X-Slack-Request-Timestamp header")
	}

	ts, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("slack verify: invalid timestamp %q: %w", tsStr, err)
	}

	age := time.Duration(math.Abs(float64(time.Now().Unix()-ts))) * time.Second
	if age > v.maxTimestampAge {
		return fmt.Errorf("slack verify: timestamp too old (%s)", age)
	}

	sig := header.Get("X-Slack-Signature")
	if sig == "" {
		return errors.New("slack verify: missing X-Slack-Signature header")
	}

	// sig format: "v0=<hex>"
	basestring := fmt.Sprintf("v0:%s:%s", tsStr, body)
	mac := hmac.New(sha256.New, []byte(v.signingSecret))
	mac.Write([]byte(basestring))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expected), []byte(sig)) {
		return errors.New("slack verify: signature mismatch")
	}

	return nil
}

// noopVerifier skips signature verification. Use only in development or
// when the signing secret is not configured.
type noopVerifier struct{}

// NewNoopVerifier returns a verifier that accepts all requests.
// Log a warning at startup when using this.
func NewNoopVerifier() SignatureVerifier {
	return &noopVerifier{}
}

func (v *noopVerifier) Verify(_ http.Header, _ []byte) error {
	return nil
}

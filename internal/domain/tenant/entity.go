package tenant

import (
	"fmt"
	"regexp"
	"time"

	"github.com/gosuda/steerlane/internal/domain"
)

// slugPattern enforces lowercase alphanumeric characters and hyphens.
// Must start and end with alphanumeric, 2-63 chars total.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$`)

// Tenant represents a multi-tenant organization.
type Tenant struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Settings  map[string]any
	Name      string
	Slug      string
	ID        domain.TenantID
}

// Validate checks that the tenant's fields are well-formed.
func (t *Tenant) Validate() error {
	if t.Name == "" {
		return fmt.Errorf("tenant name: %w", domain.ErrInvalidInput)
	}
	if !slugPattern.MatchString(t.Slug) {
		return fmt.Errorf("tenant slug %q must be lowercase alphanumeric with hyphens, 2-63 chars: %w", t.Slug, domain.ErrInvalidInput)
	}
	return nil
}

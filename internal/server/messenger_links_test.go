package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gosuda/steerlane/internal/auth"
	"github.com/gosuda/steerlane/internal/server/reqctx"
	"github.com/gosuda/steerlane/internal/store/postgres/sqlc"
)

type fakeMessengerLinkQueries struct {
	byID          map[uuid.UUID]sqlc.UserMessengerLink
	byExternalKey map[string]uuid.UUID
}

func newFakeMessengerLinkQueries() *fakeMessengerLinkQueries {
	return &fakeMessengerLinkQueries{
		byID:          make(map[uuid.UUID]sqlc.UserMessengerLink),
		byExternalKey: make(map[string]uuid.UUID),
	}
}

func (f *fakeMessengerLinkQueries) CreateMessengerLink(_ context.Context, arg sqlc.CreateMessengerLinkParams) (sqlc.UserMessengerLink, error) {
	link := sqlc.UserMessengerLink{
		ID:         uuid.New(),
		UserID:     arg.UserID,
		TenantID:   arg.TenantID,
		Platform:   arg.Platform,
		ExternalID: arg.ExternalID,
		CreatedAt:  time.Now().UTC(),
	}
	f.byID[link.ID] = link
	f.byExternalKey[arg.Platform+":"+arg.ExternalID+":"+arg.TenantID.String()] = link.ID
	return link, nil
}

func (f *fakeMessengerLinkQueries) DeleteMessengerLinkByID(_ context.Context, arg sqlc.DeleteMessengerLinkByIDParams) (sqlc.UserMessengerLink, error) {
	link, ok := f.byID[arg.ID]
	if !ok || link.UserID != arg.UserID || link.TenantID != arg.TenantID {
		return sqlc.UserMessengerLink{}, pgx.ErrNoRows
	}
	delete(f.byID, arg.ID)
	delete(f.byExternalKey, link.Platform+":"+link.ExternalID+":"+link.TenantID.String())
	return link, nil
}

func (f *fakeMessengerLinkQueries) GetMessengerLink(_ context.Context, arg sqlc.GetMessengerLinkParams) (sqlc.UserMessengerLink, error) {
	id, ok := f.byExternalKey[arg.Platform+":"+arg.ExternalID+":"+arg.TenantID.String()]
	if !ok {
		return sqlc.UserMessengerLink{}, pgx.ErrNoRows
	}
	return f.byID[id], nil
}

func (f *fakeMessengerLinkQueries) ListMessengerLinksByUser(_ context.Context, arg sqlc.ListMessengerLinksByUserParams) ([]sqlc.UserMessengerLink, error) {
	items := make([]sqlc.UserMessengerLink, 0)
	for _, link := range f.byID {
		if link.UserID == arg.UserID && link.TenantID == arg.TenantID {
			items = append(items, link)
		}
	}
	return items, nil
}

func testServerLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func makeLinkRequestContext(tenantID, userID uuid.UUID) context.Context {
	ctx := context.Background()
	ctx = reqctx.WithTenant(ctx, tenantID)
	return reqctx.WithUser(ctx, userID, "member")
}

func TestHandleMessengerLinkCompleteCreatesLink(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	userID := uuid.New()
	queries := newFakeMessengerLinkQueries()
	linking := auth.NewLinkingService("secret", "https://steerlane.example.com", time.Hour)
	token, err := linking.IssueToken(tenantID, "slack", "U123")
	require.NoError(t, err)

	srv := &Server{logger: testServerLogger(), deps: Dependencies{Links: queries, Linking: linking}}
	body, err := json.Marshal(completeMessengerLinkRequest{Token: token})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/link/complete", bytes.NewReader(body)).WithContext(makeLinkRequestContext(tenantID, userID))
	rr := httptest.NewRecorder()

	srv.handleMessengerLinkComplete(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code)
	var payload messengerLinkResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &payload))
	assert.Equal(t, "slack", payload.Platform)
	assert.Equal(t, "U123", payload.ExternalID)
	assert.Equal(t, "linked", payload.Status)
	require.Len(t, queries.byID, 1)
}

func TestHandleMessengerLinkCompleteReturnsConflictForOtherUser(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	userID := uuid.New()
	otherUserID := uuid.New()
	queries := newFakeMessengerLinkQueries()
	_, err := queries.CreateMessengerLink(context.Background(), sqlc.CreateMessengerLinkParams{
		UserID:     otherUserID,
		TenantID:   tenantID,
		Platform:   "slack",
		ExternalID: "U123",
	})
	require.NoError(t, err)

	linking := auth.NewLinkingService("secret", "https://steerlane.example.com", time.Hour)
	token, err := linking.IssueToken(tenantID, "slack", "U123")
	require.NoError(t, err)

	srv := &Server{logger: testServerLogger(), deps: Dependencies{Links: queries, Linking: linking}}
	body, err := json.Marshal(completeMessengerLinkRequest{Token: token})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/link/complete", bytes.NewReader(body)).WithContext(makeLinkRequestContext(tenantID, userID))
	rr := httptest.NewRecorder()

	srv.handleMessengerLinkComplete(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code)
}

func TestHandleMessengerLinksDeleteScopesByUser(t *testing.T) {
	t.Parallel()

	tenantID := uuid.New()
	userID := uuid.New()
	queries := newFakeMessengerLinkQueries()
	link, err := queries.CreateMessengerLink(context.Background(), sqlc.CreateMessengerLinkParams{
		UserID:     userID,
		TenantID:   tenantID,
		Platform:   "slack",
		ExternalID: "U123",
	})
	require.NoError(t, err)

	srv := &Server{logger: testServerLogger(), deps: Dependencies{Links: queries}}
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/messenger-links/"+link.ID.String(), http.NoBody)
	req.SetPathValue("id", link.ID.String())
	req = req.WithContext(makeLinkRequestContext(tenantID, userID))
	rr := httptest.NewRecorder()

	srv.handleMessengerLinksDelete(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Empty(t, queries.byID)
}

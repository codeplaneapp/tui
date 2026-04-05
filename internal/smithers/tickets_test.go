package smithers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- GetTicket ---

func TestGetTicket_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticket/get/eng-login", r.URL.Path)
		assert.Equal(t, "GET", r.Method)
		writeEnvelope(t, w, Ticket{ID: "eng-login", Content: "# Login\nImplement login flow."})
	})

	ticket, err := c.GetTicket(context.Background(), "eng-login")
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "eng-login", ticket.ID)
	assert.Equal(t, "# Login\nImplement login flow.", ticket.Content)
}

func TestGetTicket_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"ticket", "get", "eng-login", "--format", "json"}, args)
		return json.Marshal(Ticket{ID: "eng-login", Content: "# Login"})
	})

	ticket, err := c.GetTicket(context.Background(), "eng-login")
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "eng-login", ticket.ID)
}

func TestGetTicket_EmptyID(t *testing.T) {
	c := NewClient()
	_, err := c.GetTicket(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ticketID must not be empty")
}

func TestGetTicket_HTTP_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		env := apiEnvelope{OK: false, Error: "TICKET_NOT_FOUND"}
		_ = json.NewEncoder(w).Encode(env)
	})

	_, err := c.GetTicket(context.Background(), "missing-ticket")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

func TestGetTicket_Exec_NotFound(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers ticket get no-such-ticket: TICKET_NOT_FOUND")
	})

	_, err := c.GetTicket(context.Background(), "no-such-ticket")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

func TestGetTicket_Exec_NotFound_Generic(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("404 not found")
	})

	_, err := c.GetTicket(context.Background(), "missing")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

func TestGetTicket_MalformedResponse(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("not-json"), nil
	})

	_, err := c.GetTicket(context.Background(), "eng-x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ticket")
}

func TestGetTicket_MissingID_InResponse(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		// Valid JSON object but no "id" field
		return []byte(`{"content":"some content"}`), nil
	})

	_, err := c.GetTicket(context.Background(), "eng-x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id field")
}

// --- CreateTicket ---

func TestCreateTicket_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticket/create", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body CreateTicketInput
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "feat-new-feature", body.ID)
		assert.Equal(t, "# New Feature\nDescription here.", body.Content)

		writeEnvelope(t, w, Ticket{ID: "feat-new-feature", Content: "# New Feature\nDescription here."})
	})

	ticket, err := c.CreateTicket(context.Background(), CreateTicketInput{
		ID:      "feat-new-feature",
		Content: "# New Feature\nDescription here.",
	})
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "feat-new-feature", ticket.ID)
}

func TestCreateTicket_Exec_WithContent(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "ticket", args[0])
		assert.Equal(t, "create", args[1])
		assert.Equal(t, "eng-auth", args[2])
		assert.Contains(t, args, "--content")
		assert.Contains(t, args, "# Auth Ticket")
		return json.Marshal(Ticket{ID: "eng-auth", Content: "# Auth Ticket"})
	})

	ticket, err := c.CreateTicket(context.Background(), CreateTicketInput{
		ID:      "eng-auth",
		Content: "# Auth Ticket",
	})
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "eng-auth", ticket.ID)
}

func TestCreateTicket_Exec_WithoutContent(t *testing.T) {
	// When no content is provided, the --content flag should NOT appear in args.
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"ticket", "create", "eng-bare", "--format", "json"}, args)
		return json.Marshal(Ticket{ID: "eng-bare", Content: "# eng-bare\n\nTemplate content."})
	})

	ticket, err := c.CreateTicket(context.Background(), CreateTicketInput{ID: "eng-bare"})
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "eng-bare", ticket.ID)
}

func TestCreateTicket_EmptyID(t *testing.T) {
	c := NewClient()
	_, err := c.CreateTicket(context.Background(), CreateTicketInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ID must not be empty")
}

func TestCreateTicket_HTTP_Exists(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		env := apiEnvelope{OK: false, Error: "TICKET_EXISTS"}
		_ = json.NewEncoder(w).Encode(env)
	})

	_, err := c.CreateTicket(context.Background(), CreateTicketInput{ID: "dup-ticket"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketExists))
}

func TestCreateTicket_Exec_Exists(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("smithers ticket create dup-ticket: TICKET_EXISTS")
	})

	_, err := c.CreateTicket(context.Background(), CreateTicketInput{ID: "dup-ticket"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketExists))
}

func TestCreateTicket_Exec_AlreadyExists_Generic(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("409 already exists")
	})

	_, err := c.CreateTicket(context.Background(), CreateTicketInput{ID: "dup"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketExists))
}

// --- UpdateTicket ---

func TestUpdateTicket_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticket/update/eng-login", r.URL.Path)
		assert.Equal(t, "POST", r.Method)

		var body UpdateTicketInput
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "# Login (updated)", body.Content)

		writeEnvelope(t, w, Ticket{ID: "eng-login", Content: "# Login (updated)"})
	})

	ticket, err := c.UpdateTicket(context.Background(), "eng-login", UpdateTicketInput{
		Content: "# Login (updated)",
	})
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "eng-login", ticket.ID)
	assert.Equal(t, "# Login (updated)", ticket.Content)
}

func TestUpdateTicket_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, "ticket", args[0])
		assert.Equal(t, "update", args[1])
		assert.Equal(t, "eng-login", args[2])
		assert.Equal(t, "--content", args[3])
		assert.Equal(t, "# Login updated", args[4])
		return json.Marshal(Ticket{ID: "eng-login", Content: "# Login updated"})
	})

	ticket, err := c.UpdateTicket(context.Background(), "eng-login", UpdateTicketInput{
		Content: "# Login updated",
	})
	require.NoError(t, err)
	require.NotNil(t, ticket)
	assert.Equal(t, "eng-login", ticket.ID)
}

func TestUpdateTicket_EmptyID(t *testing.T) {
	c := NewClient()
	_, err := c.UpdateTicket(context.Background(), "", UpdateTicketInput{Content: "some content"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ticketID must not be empty")
}

func TestUpdateTicket_EmptyContent(t *testing.T) {
	c := NewClient()
	_, err := c.UpdateTicket(context.Background(), "eng-login", UpdateTicketInput{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Content must not be empty")
}

func TestUpdateTicket_HTTP_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		env := apiEnvelope{OK: false, Error: "TICKET_NOT_FOUND"}
		_ = json.NewEncoder(w).Encode(env)
	})

	_, err := c.UpdateTicket(context.Background(), "ghost", UpdateTicketInput{Content: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

func TestUpdateTicket_Exec_NotFound(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("TICKET_NOT_FOUND")
	})

	_, err := c.UpdateTicket(context.Background(), "ghost", UpdateTicketInput{Content: "x"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

// --- DeleteTicket ---

func TestDeleteTicket_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticket/delete/eng-login", r.URL.Path)
		assert.Equal(t, "POST", r.Method)
		writeEnvelope(t, w, nil)
	})

	err := c.DeleteTicket(context.Background(), "eng-login")
	require.NoError(t, err)
}

func TestDeleteTicket_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"ticket", "delete", "eng-login"}, args)
		return nil, nil
	})

	err := c.DeleteTicket(context.Background(), "eng-login")
	require.NoError(t, err)
}

func TestDeleteTicket_EmptyID(t *testing.T) {
	c := NewClient()
	err := c.DeleteTicket(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ticketID must not be empty")
}

func TestDeleteTicket_HTTP_NotFound(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		env := apiEnvelope{OK: false, Error: "TICKET_NOT_FOUND"}
		_ = json.NewEncoder(w).Encode(env)
	})

	err := c.DeleteTicket(context.Background(), "ghost")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

func TestDeleteTicket_Exec_NotFound(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("TICKET_NOT_FOUND: ghost")
	})

	err := c.DeleteTicket(context.Background(), "ghost")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTicketNotFound))
}

func TestDeleteTicket_Exec_Error(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("unexpected exec failure")
	})

	err := c.DeleteTicket(context.Background(), "eng-login")
	require.Error(t, err)
	assert.NotEqual(t, ErrTicketNotFound, err)
}

// --- SearchTickets ---

func TestSearchTickets_HTTP(t *testing.T) {
	_, c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/ticket/search", r.URL.Path)
		assert.Equal(t, "login", r.URL.Query().Get("q"))
		writeEnvelope(t, w, []Ticket{
			{ID: "eng-login", Content: "# Login flow"},
			{ID: "eng-logout", Content: "# Logout flow"},
		})
	})

	tickets, err := c.SearchTickets(context.Background(), "login")
	require.NoError(t, err)
	require.Len(t, tickets, 2)
	assert.Equal(t, "eng-login", tickets[0].ID)
}

func TestSearchTickets_Exec(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		assert.Equal(t, []string{"ticket", "search", "login", "--format", "json"}, args)
		return json.Marshal([]Ticket{
			{ID: "eng-login", Content: "# Login"},
		})
	})

	tickets, err := c.SearchTickets(context.Background(), "login")
	require.NoError(t, err)
	require.Len(t, tickets, 1)
	assert.Equal(t, "eng-login", tickets[0].ID)
}

func TestSearchTickets_EmptyQuery(t *testing.T) {
	c := NewClient()
	_, err := c.SearchTickets(context.Background(), "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "query must not be empty")
}

func TestSearchTickets_Exec_NoResults(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return json.Marshal([]Ticket{})
	})

	tickets, err := c.SearchTickets(context.Background(), "nonexistent")
	require.NoError(t, err)
	assert.Empty(t, tickets)
}

func TestSearchTickets_Exec_MalformedResponse(t *testing.T) {
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		return []byte("{bad json"), nil
	})

	_, err := c.SearchTickets(context.Background(), "query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ticket search results")
}

// --- Transport fallback: exec is tried when no server is configured ---

func TestGetTicket_TransportFallback(t *testing.T) {
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		assert.Equal(t, "ticket", args[0])
		assert.Equal(t, "get", args[1])
		return json.Marshal(Ticket{ID: "eng-x", Content: "body"})
	})

	ticket, err := c.GetTicket(context.Background(), "eng-x")
	require.NoError(t, err)
	assert.True(t, execCalled, "should fall through to exec when no server configured")
	assert.Equal(t, "eng-x", ticket.ID)
}

func TestCreateTicket_TransportFallback(t *testing.T) {
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		return json.Marshal(Ticket{ID: "eng-y", Content: ""})
	})

	ticket, err := c.CreateTicket(context.Background(), CreateTicketInput{ID: "eng-y"})
	require.NoError(t, err)
	assert.True(t, execCalled)
	assert.Equal(t, "eng-y", ticket.ID)
}

func TestUpdateTicket_TransportFallback(t *testing.T) {
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		return json.Marshal(Ticket{ID: "eng-z", Content: "updated"})
	})

	ticket, err := c.UpdateTicket(context.Background(), "eng-z", UpdateTicketInput{Content: "updated"})
	require.NoError(t, err)
	assert.True(t, execCalled)
	assert.Equal(t, "eng-z", ticket.ID)
}

func TestDeleteTicket_TransportFallback(t *testing.T) {
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		return nil, nil
	})

	err := c.DeleteTicket(context.Background(), "eng-z")
	require.NoError(t, err)
	assert.True(t, execCalled)
}

func TestSearchTickets_TransportFallback(t *testing.T) {
	execCalled := false
	c := newExecClient(func(_ context.Context, args ...string) ([]byte, error) {
		execCalled = true
		return json.Marshal([]Ticket{{ID: "eng-found", Content: "match"}})
	})

	tickets, err := c.SearchTickets(context.Background(), "match")
	require.NoError(t, err)
	assert.True(t, execCalled)
	assert.Len(t, tickets, 1)
}

// --- Sentinel error classification helpers ---

func TestIsTicketNotFoundErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"TICKET_NOT_FOUND", fmt.Errorf("TICKET_NOT_FOUND"), true},
		{"ticket_not_found lowercase", fmt.Errorf("ticket_not_found"), true},
		{"not found generic", fmt.Errorf("not found"), true},
		{"404 string", fmt.Errorf("404"), true},
		{"smithers API 404 message", fmt.Errorf("smithers API error: 404 not found"), true},
		{"unrelated error", fmt.Errorf("connection refused"), false},
		{"TICKET_EXISTS", fmt.Errorf("TICKET_EXISTS"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTicketNotFoundErr(tt.err))
		})
	}
}

func TestIsTicketExistsErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"TICKET_EXISTS", fmt.Errorf("TICKET_EXISTS"), true},
		{"ticket_exists lowercase", fmt.Errorf("ticket_exists"), true},
		{"already exists generic", fmt.Errorf("already exists"), true},
		{"409 string", fmt.Errorf("409"), true},
		{"smithers API 409 message", fmt.Errorf("smithers API error: 409 already exists"), true},
		{"unrelated error", fmt.Errorf("connection refused"), false},
		{"TICKET_NOT_FOUND", fmt.Errorf("TICKET_NOT_FOUND"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTicketExistsErr(tt.err))
		})
	}
}

// --- parseTicketJSON ---

func TestParseTicketJSON_Valid(t *testing.T) {
	data, _ := json.Marshal(Ticket{ID: "t1", Content: "body"})
	ticket, err := parseTicketJSON(data)
	require.NoError(t, err)
	assert.Equal(t, "t1", ticket.ID)
}

func TestParseTicketJSON_BadJSON(t *testing.T) {
	_, err := parseTicketJSON([]byte("not json"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ticket")
}

func TestParseTicketJSON_MissingID(t *testing.T) {
	_, err := parseTicketJSON([]byte(`{"content":"body"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id field")
}

// --- parseTicketsSearchResultJSON ---

func TestParseTicketsSearchResultJSON_Valid(t *testing.T) {
	data, _ := json.Marshal([]Ticket{{ID: "a"}, {ID: "b"}})
	tickets, err := parseTicketsSearchResultJSON(data)
	require.NoError(t, err)
	assert.Len(t, tickets, 2)
}

func TestParseTicketsSearchResultJSON_Empty(t *testing.T) {
	tickets, err := parseTicketsSearchResultJSON([]byte("[]"))
	require.NoError(t, err)
	assert.Empty(t, tickets)
}

func TestParseTicketsSearchResultJSON_BadJSON(t *testing.T) {
	_, err := parseTicketsSearchResultJSON([]byte("{not an array}"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse ticket search results")
}

package smithers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Ticket domain sentinel errors. Callers can test with errors.Is.
var (
	// ErrTicketNotFound is returned when a requested ticket does not exist.
	ErrTicketNotFound = errors.New("ticket not found")
	// ErrTicketExists is returned when creating a ticket whose ID already exists.
	ErrTicketExists = errors.New("ticket already exists")
)

// GetTicket retrieves a single ticket by ID.
// Routes: HTTP GET /ticket/get/<id> → exec smithers ticket get <id>.
func (c *Client) GetTicket(ctx context.Context, ticketID string) (*Ticket, error) {
	if ticketID == "" {
		return nil, fmt.Errorf("ticketID must not be empty")
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var ticket Ticket
		err := c.httpGetJSON(ctx, "/ticket/get/"+ticketID, &ticket)
		if err == nil {
			return &ticket, nil
		}
		if isTicketNotFoundErr(err) {
			return nil, ErrTicketNotFound
		}
	}

	// 2. Fall back to exec
	out, err := c.execSmithers(ctx, "ticket", "get", ticketID, "--format", "json")
	if err != nil {
		if isTicketNotFoundErr(err) {
			return nil, ErrTicketNotFound
		}
		return nil, err
	}
	return parseTicketJSON(out)
}

// CreateTicket creates a new ticket with the given ID and optional content.
// Routes: HTTP POST /ticket/create → exec smithers ticket create <id> [--content <content>].
func (c *Client) CreateTicket(ctx context.Context, input CreateTicketInput) (*Ticket, error) {
	if input.ID == "" {
		return nil, fmt.Errorf("CreateTicketInput.ID must not be empty")
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var ticket Ticket
		err := c.httpPostJSON(ctx, "/ticket/create", input, &ticket)
		if err == nil {
			return &ticket, nil
		}
		if isTicketExistsErr(err) {
			return nil, ErrTicketExists
		}
	}

	// 2. Fall back to exec
	args := []string{"ticket", "create", input.ID, "--format", "json"}
	if input.Content != "" {
		args = append(args, "--content", input.Content)
	}
	out, err := c.execSmithers(ctx, args...)
	if err != nil {
		if isTicketExistsErr(err) {
			return nil, ErrTicketExists
		}
		return nil, err
	}
	return parseTicketJSON(out)
}

// UpdateTicket replaces the content of an existing ticket.
// Routes: HTTP POST /ticket/update/<id> → exec smithers ticket update <id> --content <content>.
func (c *Client) UpdateTicket(ctx context.Context, ticketID string, input UpdateTicketInput) (*Ticket, error) {
	if ticketID == "" {
		return nil, fmt.Errorf("ticketID must not be empty")
	}
	if input.Content == "" {
		return nil, fmt.Errorf("UpdateTicketInput.Content must not be empty")
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var ticket Ticket
		err := c.httpPostJSON(ctx, "/ticket/update/"+ticketID, input, &ticket)
		if err == nil {
			return &ticket, nil
		}
		if isTicketNotFoundErr(err) {
			return nil, ErrTicketNotFound
		}
	}

	// 2. Fall back to exec
	out, err := c.execSmithers(ctx, "ticket", "update", ticketID, "--content", input.Content, "--format", "json")
	if err != nil {
		if isTicketNotFoundErr(err) {
			return nil, ErrTicketNotFound
		}
		return nil, err
	}
	return parseTicketJSON(out)
}

// DeleteTicket removes a ticket by ID.
// Routes: HTTP POST /ticket/delete/<id> → exec smithers ticket delete <id>.
func (c *Client) DeleteTicket(ctx context.Context, ticketID string) error {
	if ticketID == "" {
		return fmt.Errorf("ticketID must not be empty")
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		err := c.httpPostJSON(ctx, "/ticket/delete/"+ticketID, nil, nil)
		if err == nil {
			return nil
		}
		if isTicketNotFoundErr(err) {
			return ErrTicketNotFound
		}
	}

	// 2. Fall back to exec
	_, err := c.execSmithers(ctx, "ticket", "delete", ticketID)
	if err != nil {
		if isTicketNotFoundErr(err) {
			return ErrTicketNotFound
		}
		return err
	}
	return nil
}

// SearchTickets returns tickets whose ID or content contains the query string.
// Routes: HTTP GET /ticket/search?q=<query> → exec smithers ticket search <query>.
// The search is case-insensitive and performed server-side; results are ordered
// by relevance as determined by the upstream implementation.
func (c *Client) SearchTickets(ctx context.Context, query string) ([]Ticket, error) {
	if query == "" {
		return nil, fmt.Errorf("query must not be empty")
	}

	// 1. Try HTTP
	if c.isServerAvailable() {
		var tickets []Ticket
		err := c.httpGetJSON(ctx, "/ticket/search?q="+query, &tickets)
		if err == nil {
			return tickets, nil
		}
	}

	// 2. Fall back to exec
	out, err := c.execSmithers(ctx, "ticket", "search", query, "--format", "json")
	if err != nil {
		return nil, err
	}
	return parseTicketsSearchResultJSON(out)
}

// --- Parse helpers ---

// parseTicketJSON parses exec/HTTP output into a single Ticket.
func parseTicketJSON(data []byte) (*Ticket, error) {
	var t Ticket
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parse ticket: %w", err)
	}
	if t.ID == "" {
		return nil, fmt.Errorf("parse ticket: missing id field in response")
	}
	return &t, nil
}

// parseTicketsSearchResultJSON parses exec output into a Ticket slice for search results.
func parseTicketsSearchResultJSON(data []byte) ([]Ticket, error) {
	var tickets []Ticket
	if err := json.Unmarshal(data, &tickets); err != nil {
		return nil, fmt.Errorf("parse ticket search results: %w", err)
	}
	return tickets, nil
}

// isTicketNotFoundErr reports whether err signals a missing ticket.
// Matches TICKET_NOT_FOUND (upstream CLI string), HTTP 404, or generic "not found".
func isTicketNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "TICKET_NOT_FOUND") ||
		strings.Contains(msg, "NOT FOUND") ||
		strings.Contains(msg, "404")
}

// isTicketExistsErr reports whether err signals a duplicate ticket ID.
// Matches TICKET_EXISTS (upstream CLI string), HTTP 409, or generic "already exists".
func isTicketExistsErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "TICKET_EXISTS") ||
		strings.Contains(msg, "ALREADY EXISTS") ||
		strings.Contains(msg, "409")
}

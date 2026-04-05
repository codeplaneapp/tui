package smithers

// CreateTicketInput holds the fields required to create a new ticket.
// Maps to the smithers CLI: smithers ticket create <id> [--content <content>]
type CreateTicketInput struct {
	// ID is the ticket identifier / filename slug (e.g. "feat-login-flow").
	// Required — the Smithers CLI uses this as the file name under .smithers/tickets/.
	ID string `json:"id"`

	// Content is the full markdown body of the ticket.
	// Optional: when omitted, the upstream CLI generates a default template.
	Content string `json:"content,omitempty"`
}

// UpdateTicketInput holds the fields that may be patched on an existing ticket.
// Only non-zero fields are sent to the server / CLI.
type UpdateTicketInput struct {
	// Content replaces the full markdown body of the ticket. Required.
	Content string `json:"content"`
}

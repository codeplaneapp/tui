package views

import (
	"testing"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/stretchr/testify/assert"
)

func TestNewDashboardView_HidesTimelineMenuItem(t *testing.T) {
	t.Parallel()

	view := NewDashboardView(smithers.NewClient(), true)

	var labels []string
	for _, item := range view.menuItems {
		labels = append(labels, item.label)
	}

	assert.NotContains(t, labels, "Timeline")
	assert.Contains(t, labels, "Tickets")
}

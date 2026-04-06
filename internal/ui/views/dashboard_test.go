package views

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewDashboardView_HidesTimelineMenuItem(t *testing.T) {
	v := NewDashboardView(nil, true)

	labels := make([]string, 0, len(v.menuItems))
	for _, item := range v.menuItems {
		labels = append(labels, item.label)
	}

	assert.Contains(t, labels, "Work Items")
	assert.NotContains(t, labels, "Timeline")
}

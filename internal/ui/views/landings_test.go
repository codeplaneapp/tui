package views

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/styles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleLanding(number int, state, title string) jjhub.Landing {
	return jjhub.Landing{
		Number:         number,
		Title:          title,
		Body:           "## Summary\n\n" + title,
		State:          state,
		TargetBookmark: "main",
		ChangeIDs:      []string{"abc123", "def456"},
		StackSize:      2,
		ConflictStatus: "clean",
		Author:         jjhub.User{Login: "will"},
		CreatedAt:      time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		UpdatedAt:      time.Now().Add(-30 * time.Minute).Format(time.RFC3339),
	}
}

func newTestLandingsView() *LandingsView {
	return NewLandingsView(smithers.NewClient())
}

func seedLandingsView(v *LandingsView, landings []jjhub.Landing) *LandingsView {
	updated, _ := v.Update(landingsLoadedMsg{landings: landings})
	return updated.(*LandingsView)
}

func TestLandingsView_ImplementsView(t *testing.T) {
	t.Parallel()
	var _ View = (*LandingsView)(nil)
}

func TestLandingsView_FilterCycle(t *testing.T) {
	t.Parallel()

	v := seedLandingsView(newTestLandingsView(), []jjhub.Landing{
		sampleLanding(1, "open", "Open landing"),
		sampleLanding(2, "merged", "Merged landing"),
	})

	updated, _ := v.Update(tea.KeyPressMsg{Code: 's'})
	lv := updated.(*LandingsView)

	assert.Equal(t, "merged", lv.currentFilter())
	assert.Len(t, lv.landings, 1)
	assert.Equal(t, 2, lv.landings[0].Number)
}

func TestLandingsView_SearchApply(t *testing.T) {
	t.Parallel()

	v := seedLandingsView(newTestLandingsView(), []jjhub.Landing{
		sampleLanding(1, "open", "Alpha"),
		sampleLanding(2, "open", "Beta"),
	})
	v.search.active = true
	v.search.input.SetValue("beta")

	updated, _ := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	lv := updated.(*LandingsView)

	assert.Equal(t, "beta", lv.searchQuery)
	assert.Len(t, lv.landings, 1)
	assert.Equal(t, "Beta", lv.landings[0].Title)
}

func TestLandingsView_WTogglesPreview(t *testing.T) {
	t.Parallel()

	v := seedLandingsView(newTestLandingsView(), []jjhub.Landing{sampleLanding(1, "open", "Alpha")})
	assert.True(t, v.previewOpen)

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'w'})
	lv := updated.(*LandingsView)

	assert.False(t, lv.previewOpen)
}

func TestLandingsView_EnterReturnsDetailView(t *testing.T) {
	t.Parallel()

	v := seedLandingsView(newTestLandingsView(), []jjhub.Landing{sampleLanding(1, "open", "Alpha")})
	v.width = 120
	v.height = 40

	updated, cmd := v.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	require.IsType(t, &LandingDetailView{}, updated)
	require.NotNil(t, cmd)
}

func TestLandingDetailView_EscReturnsParent(t *testing.T) {
	t.Parallel()

	parent := seedLandingsView(newTestLandingsView(), []jjhub.Landing{sampleLanding(1, "open", "Alpha")})
	detail := NewLandingDetailView(parent, jjhub.NewClient(""), nil, styles.DefaultStyles(), sampleLanding(1, "open", "Alpha"), nil, nil)
	detail.SetSize(120, 40)

	updated, cmd := detail.Update(tea.KeyPressMsg{Code: tea.KeyEscape})

	require.Nil(t, cmd)
	assert.Same(t, parent, updated)
}

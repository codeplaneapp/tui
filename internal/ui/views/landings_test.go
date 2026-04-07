package views

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/crush/internal/jjhub"
	"github.com/charmbracelet/crush/internal/observability"
	"github.com/stretchr/testify/require"
)

type mockLandingManager struct {
	landings []jjhub.Landing
	checks   string

	reviewCalls []struct {
		number int
		action string
		body   string
	}
	landCalls []int
}

func (m *mockLandingManager) GetCurrentRepo(context.Context) (*jjhub.Repo, error) {
	return &jjhub.Repo{FullName: "acme/repo"}, nil
}

func (m *mockLandingManager) ListLandings(context.Context, string, int) ([]jjhub.Landing, error) {
	return append([]jjhub.Landing(nil), m.landings...), nil
}

func (m *mockLandingManager) ViewLanding(context.Context, int) (*jjhub.LandingDetail, error) {
	return &jjhub.LandingDetail{}, nil
}

func (m *mockLandingManager) CreateLanding(context.Context, string, string, string, bool) (*jjhub.Landing, error) {
	return &jjhub.Landing{Number: 99, Title: "new landing"}, nil
}

func (m *mockLandingManager) ReviewLanding(_ context.Context, number int, action, body string) error {
	m.reviewCalls = append(m.reviewCalls, struct {
		number int
		action string
		body   string
	}{number: number, action: action, body: body})
	return nil
}

func (m *mockLandingManager) LandLanding(_ context.Context, number int) error {
	m.landCalls = append(m.landCalls, number)
	return nil
}

func (m *mockLandingManager) LandingDiff(context.Context, int) (string, error) {
	return "diff --git a/main.go b/main.go", nil
}

func (m *mockLandingManager) LandingChecks(context.Context, int) (string, error) {
	return m.checks, nil
}

func configureLandingsObservability(t *testing.T) {
	t.Helper()

	t.Cleanup(func() {
		require.NoError(t, observability.Shutdown(context.Background()))
	})

	require.NoError(t, observability.Configure(context.Background(), observability.Config{
		ServiceName:      "test",
		ServiceVersion:   "dev",
		Mode:             observability.ModeLocal,
		TraceBufferSize:  32,
		TraceSampleRatio: 1,
	}))
}

func requireLandingsSpanAttrs(t *testing.T, action, result string) map[string]any {
	t.Helper()

	spans := observability.RecentSpans(30)
	for i := len(spans) - 1; i >= 0; i-- {
		span := spans[i]
		if span.Name != "ui.action" {
			continue
		}
		if span.Attributes["codeplane.ui.view"] == "landings" &&
			span.Attributes["codeplane.ui.action"] == action &&
			span.Attributes["codeplane.ui.result"] == result {
			return span.Attributes
		}
	}

	t.Fatalf("missing landings ui.action span action=%q result=%q", action, result)
	return nil
}

func testLanding(number int, title string) jjhub.Landing {
	return jjhub.Landing{
		Number: number,
		Title:  title,
		State:  "open",
		Author: jjhub.User{Login: "acme"},
	}
}

func TestLandingsView_StateFilterCycleRecordsAction(t *testing.T) {
	configureLandingsObservability(t)

	manager := &mockLandingManager{}
	v := newLandingsViewWithClient(manager)

	updated, cmd := v.Update(tea.KeyPressMsg{Code: 's'})
	lv := updated.(*LandingsView)

	require.Equal(t, "draft", lv.stateFilter)
	require.NotNil(t, cmd)

	attrs := requireLandingsSpanAttrs(t, "set_state_filter", "ok")
	require.Equal(t, "draft", attrs["codeplane.landings.state_filter"])
}

func TestLandingsView_ChecksToggleUsesTAndKStillMoves(t *testing.T) {
	configureLandingsObservability(t)

	manager := &mockLandingManager{
		landings: []jjhub.Landing{
			testLanding(1, "first"),
			testLanding(2, "second"),
		},
		checks: "ci: success",
	}
	v := newLandingsViewWithClient(manager)
	v.landings = manager.landings
	v.cursor = 1

	updated, _ := v.Update(tea.KeyPressMsg{Code: 'k'})
	lv := updated.(*LandingsView)
	require.Equal(t, 0, lv.cursor)
	require.Equal(t, landingPanelDetail, lv.panel)

	updated, cmd := lv.Update(tea.KeyPressMsg{Code: 't'})
	lv = updated.(*LandingsView)
	require.Equal(t, landingPanelChecks, lv.panel)
	require.NotNil(t, cmd)

	msg := cmd()
	updated, _ = lv.Update(msg)
	lv = updated.(*LandingsView)
	require.Equal(t, "ci: success", lv.checksCache[manager.landings[0].Number])

	attrs := requireLandingsSpanAttrs(t, "checks", "ok")
	require.EqualValues(t, manager.landings[0].Number, attrs["codeplane.landings.number"])
}

func TestLandingsView_RequestChangesAndCommentRecordObservability(t *testing.T) {
	configureLandingsObservability(t)

	manager := &mockLandingManager{
		landings: []jjhub.Landing{testLanding(7, "review me")},
	}
	v := newLandingsViewWithClient(manager)
	v.landings = manager.landings

	msg := v.reviewLandingCmd("request_changes", "needs work")()
	updated, _ := v.Update(msg)
	lv := updated.(*LandingsView)
	require.Equal(t, "Requested changes on landing #7", lv.actionMsg)
	require.Len(t, manager.reviewCalls, 1)
	require.Equal(t, "request_changes", manager.reviewCalls[0].action)

	attrs := requireLandingsSpanAttrs(t, "review", "ok")
	require.Equal(t, "request_changes", attrs["codeplane.landings.review_action"])

	msg = lv.reviewLandingCmd("comment", "looks close")()
	updated, _ = lv.Update(msg)
	lv = updated.(*LandingsView)
	require.Equal(t, "Commented on landing #7", lv.actionMsg)
	require.Len(t, manager.reviewCalls, 2)
	require.Equal(t, "comment", manager.reviewCalls[1].action)
}

func TestLandingsView_LandRecordsObservability(t *testing.T) {
	configureLandingsObservability(t)

	manager := &mockLandingManager{
		landings: []jjhub.Landing{testLanding(11, "ship it")},
	}
	v := newLandingsViewWithClient(manager)
	v.landings = manager.landings

	msg := v.landLandingCmd()()
	updated, _ := v.Update(msg)
	lv := updated.(*LandingsView)

	require.Equal(t, "Landed landing #11", lv.actionMsg)
	require.Equal(t, []int{11}, manager.landCalls)

	attrs := requireLandingsSpanAttrs(t, "land", "ok")
	require.EqualValues(t, 11, attrs["codeplane.landings.number"])
}

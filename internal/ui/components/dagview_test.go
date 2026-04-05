package components_test

import (
	"strings"
	"testing"

	"github.com/charmbracelet/crush/internal/smithers"
	"github.com/charmbracelet/crush/internal/ui/components"
	"github.com/stretchr/testify/assert"
)

// --- RenderDAGFields ---

func TestRenderDAGFields_Empty(t *testing.T) {
	out := components.RenderDAGFields(nil, 120)
	assert.Empty(t, out, "nil fields should produce empty output")

	out2 := components.RenderDAGFields([]smithers.WorkflowTask{}, 120)
	assert.Empty(t, out2, "empty fields slice should produce empty output")
}

func TestRenderDAGFields_SingleField(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
	}
	out := components.RenderDAGFields(fields, 120)
	assert.NotEmpty(t, out)
	// Should contain the label.
	assert.Contains(t, out, "Prompt")
	// Should contain box-drawing characters.
	assert.Contains(t, out, "┌")
	assert.Contains(t, out, "┘")
}

func TestRenderDAGFields_TwoFields_HasArrow(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "prompt", Label: "Prompt", Type: "string"},
		{Key: "ticketId", Label: "Ticket ID", Type: "string"},
	}
	out := components.RenderDAGFields(fields, 120)
	assert.Contains(t, out, "Prompt")
	assert.Contains(t, out, "Ticket ID")
	// Arrow should be present when two fields fit horizontally.
	assert.Contains(t, out, "──▶")
}

func TestRenderDAGFields_TypeAnnotations(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "count", Label: "Count", Type: "number"},
	}
	out := components.RenderDAGFields(fields, 120)
	assert.Contains(t, out, "(number)")
}

func TestRenderDAGFields_DefaultsTypeToString(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "x", Label: "X", Type: ""},
	}
	out := components.RenderDAGFields(fields, 120)
	// Should default to "string" when Type is empty.
	assert.Contains(t, out, "(string)")
}

func TestRenderDAGFields_NarrowTerminal_UsesVerticalLayout(t *testing.T) {
	// With a very narrow maxWidth, the horizontal layout won't fit.
	fields := []smithers.WorkflowTask{
		{Key: "a", Label: "Alpha", Type: "string"},
		{Key: "b", Label: "Beta", Type: "string"},
	}
	// Narrow enough to force vertical layout.
	out := components.RenderDAGFields(fields, 20)
	assert.Contains(t, out, "Alpha")
	assert.Contains(t, out, "Beta")
	// Vertical layout uses downward arrow.
	assert.Contains(t, out, "▼")
}

func TestRenderDAGFields_ZeroMaxWidth_DefaultsTo120(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "p", Label: "Prompt", Type: "string"},
	}
	out := components.RenderDAGFields(fields, 0)
	assert.NotEmpty(t, out)
	assert.Contains(t, out, "Prompt")
}

func TestRenderDAGFields_MultipleFields_AllLabelsPresent(t *testing.T) {
	fields := []smithers.WorkflowTask{
		{Key: "a", Label: "First", Type: "string"},
		{Key: "b", Label: "Second", Type: "string"},
		{Key: "c", Label: "Third", Type: "string"},
	}
	out := components.RenderDAGFields(fields, 200)
	assert.Contains(t, out, "First")
	assert.Contains(t, out, "Second")
	assert.Contains(t, out, "Third")
}

// --- RenderDAGTasks ---

func TestRenderDAGTasks_Empty(t *testing.T) {
	out := components.RenderDAGTasks(nil, 120)
	assert.Empty(t, out)

	out2 := components.RenderDAGTasks([]smithers.RunTask{}, 120)
	assert.Empty(t, out2)
}

func TestRenderDAGTasks_SingleTask_UsesNodeID(t *testing.T) {
	tasks := []smithers.RunTask{
		{NodeID: "generate", State: smithers.TaskStateRunning},
	}
	out := components.RenderDAGTasks(tasks, 120)
	assert.Contains(t, out, "generate")
}

func TestRenderDAGTasks_UsesLabelOverNodeID(t *testing.T) {
	lbl := "Generate Code"
	tasks := []smithers.RunTask{
		{NodeID: "gen-node", Label: &lbl, State: smithers.TaskStateFinished},
	}
	out := components.RenderDAGTasks(tasks, 120)
	assert.Contains(t, out, "Generate Code")
}

func TestRenderDAGTasks_TwoTasks_HasBoxes(t *testing.T) {
	tasks := []smithers.RunTask{
		{NodeID: "step1", State: smithers.TaskStateFinished},
		{NodeID: "step2", State: smithers.TaskStatePending},
	}
	out := components.RenderDAGTasks(tasks, 120)
	assert.Contains(t, out, "step1")
	assert.Contains(t, out, "step2")
	assert.Contains(t, out, "┌")
}

func TestRenderDAGTasks_NarrowTerminal_VerticalLayout(t *testing.T) {
	tasks := []smithers.RunTask{
		{NodeID: "step1", State: smithers.TaskStateFinished},
		{NodeID: "step2", State: smithers.TaskStatePending},
	}
	out := components.RenderDAGTasks(tasks, 15)
	// Vertical layout uses downward arrow.
	assert.Contains(t, out, "▼")
}

func TestRenderDAGTasks_OutputContainsBoxDrawing(t *testing.T) {
	tasks := []smithers.RunTask{
		{NodeID: "n1", State: smithers.TaskStateRunning},
	}
	out := components.RenderDAGTasks(tasks, 120)
	// Should contain box-drawing corners.
	assert.True(t, strings.Contains(out, "┌") || strings.Contains(out, "│"),
		"expected box-drawing characters in output: %q", out)
}

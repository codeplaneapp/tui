package model

import (
	"testing"

	"github.com/charmbracelet/crush/internal/session"
	"github.com/stretchr/testify/assert"
)

func TestHasInProgressTodo_Empty(t *testing.T) {
	t.Parallel()

	assert.False(t, hasInProgressTodo(nil))
	assert.False(t, hasInProgressTodo([]session.Todo{}))
}

func TestHasInProgressTodo_AllCompleted(t *testing.T) {
	t.Parallel()

	todos := []session.Todo{
		{Content: "a", Status: session.TodoStatusCompleted},
		{Content: "b", Status: session.TodoStatusCompleted},
	}
	assert.False(t, hasInProgressTodo(todos))
}

func TestHasInProgressTodo_AllPending(t *testing.T) {
	t.Parallel()

	todos := []session.Todo{
		{Content: "a", Status: session.TodoStatusPending},
		{Content: "b", Status: session.TodoStatusPending},
	}
	assert.False(t, hasInProgressTodo(todos))
}

func TestHasInProgressTodo_HasInProgress(t *testing.T) {
	t.Parallel()

	todos := []session.Todo{
		{Content: "done", Status: session.TodoStatusCompleted},
		{Content: "doing", Status: session.TodoStatusInProgress},
		{Content: "waiting", Status: session.TodoStatusPending},
	}
	assert.True(t, hasInProgressTodo(todos))
}

func TestHasInProgressTodo_OnlyInProgress(t *testing.T) {
	t.Parallel()

	todos := []session.Todo{
		{Content: "doing", Status: session.TodoStatusInProgress},
	}
	assert.True(t, hasInProgressTodo(todos))
}

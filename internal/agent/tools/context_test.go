package tools

import (
	"context"
	"testing"
)

// Test-specific context key types to avoid collisions
type (
	testStringKey string
	testBoolKey   string
	testIntKey    string
)

const (
	testKey     testStringKey = "testKey"
	missingKey  testStringKey = "missingKey"
	boolTestKey testBoolKey   = "boolKey"
	intTestKey  testIntKey    = "intKey"
)

func TestGetContextValue(t *testing.T) {
	tests := []struct {
		name         string
		setup        func(ctx context.Context) context.Context
		key          any
		defaultValue any
		want         any
	}{
		{
			name: "returns string value",
			setup: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, testKey, "testValue")
			},
			key:          testKey,
			defaultValue: "",
			want:         "testValue",
		},
		{
			name: "returns default when key not found",
			setup: func(ctx context.Context) context.Context {
				return ctx
			},
			key:          missingKey,
			defaultValue: "default",
			want:         "default",
		},
		{
			name: "returns default when type mismatch",
			setup: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, testKey, 123) // int, not string
			},
			key:          testKey,
			defaultValue: "default",
			want:         "default",
		},
		{
			name: "returns bool value",
			setup: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, boolTestKey, true)
			},
			key:          boolTestKey,
			defaultValue: false,
			want:         true,
		},
		{
			name: "returns int value",
			setup: func(ctx context.Context) context.Context {
				return context.WithValue(ctx, intTestKey, 42)
			},
			key:          intTestKey,
			defaultValue: 0,
			want:         42,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup(context.Background())

			var got any
			switch tt.defaultValue.(type) {
			case string:
				got = getContextValue(ctx, tt.key, tt.defaultValue.(string))
			case bool:
				got = getContextValue(ctx, tt.key, tt.defaultValue.(bool))
			case int:
				got = getContextValue(ctx, tt.key, tt.defaultValue.(int))
			}

			if got != tt.want {
				t.Errorf("getContextValue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSessionFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "returns session ID when present",
			ctx:  context.WithValue(context.Background(), SessionIDContextKey, "session-123"),
			want: "session-123",
		},
		{
			name: "returns empty string when not present",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "returns empty string when wrong type",
			ctx:  context.WithValue(context.Background(), SessionIDContextKey, 123),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSessionFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("GetSessionFromContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetMessageFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "returns message ID when present",
			ctx:  context.WithValue(context.Background(), MessageIDContextKey, "msg-456"),
			want: "msg-456",
		},
		{
			name: "returns empty string when not present",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "returns empty string when wrong type",
			ctx:  context.WithValue(context.Background(), MessageIDContextKey, 456),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetMessageFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("GetMessageFromContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSupportsImagesFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{
			name: "returns true when present and true",
			ctx:  context.WithValue(context.Background(), SupportsImagesContextKey, true),
			want: true,
		},
		{
			name: "returns false when present and false",
			ctx:  context.WithValue(context.Background(), SupportsImagesContextKey, false),
			want: false,
		},
		{
			name: "returns false when not present",
			ctx:  context.Background(),
			want: false,
		},
		{
			name: "returns false when wrong type",
			ctx:  context.WithValue(context.Background(), SupportsImagesContextKey, "true"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSupportsImagesFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("GetSupportsImagesFromContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContextValuesComposeCorrectly(t *testing.T) {
	// Verify that multiple context values can be set and retrieved independently.
	ctx := context.Background()
	ctx = context.WithValue(ctx, SessionIDContextKey, "sess-1")
	ctx = context.WithValue(ctx, MessageIDContextKey, "msg-2")
	ctx = context.WithValue(ctx, SupportsImagesContextKey, true)
	ctx = context.WithValue(ctx, ModelNameContextKey, "claude-opus-4")

	if got := GetSessionFromContext(ctx); got != "sess-1" {
		t.Errorf("GetSessionFromContext() = %v, want %v", got, "sess-1")
	}
	if got := GetMessageFromContext(ctx); got != "msg-2" {
		t.Errorf("GetMessageFromContext() = %v, want %v", got, "msg-2")
	}
	if got := GetSupportsImagesFromContext(ctx); got != true {
		t.Errorf("GetSupportsImagesFromContext() = %v, want %v", got, true)
	}
	if got := GetModelNameFromContext(ctx); got != "claude-opus-4" {
		t.Errorf("GetModelNameFromContext() = %v, want %v", got, "claude-opus-4")
	}
}

func TestGetContextValueWithZeroValues(t *testing.T) {
	// Test that zero values (empty string, false) are distinguishable from "not set".
	// When a key is explicitly set to the zero value, it should return the zero value,
	// not the default.
	ctx := context.WithValue(context.Background(), SessionIDContextKey, "")
	got := GetSessionFromContext(ctx)
	if got != "" {
		t.Errorf("expected empty string for explicitly-set zero value, got %q", got)
	}

	ctx2 := context.WithValue(context.Background(), SupportsImagesContextKey, false)
	got2 := GetSupportsImagesFromContext(ctx2)
	if got2 != false {
		t.Errorf("expected false for explicitly-set false value, got %v", got2)
	}
}

func TestGetModelNameFromContext(t *testing.T) {
	tests := []struct {
		name string
		ctx  context.Context
		want string
	}{
		{
			name: "returns model name when present",
			ctx:  context.WithValue(context.Background(), ModelNameContextKey, "claude-opus-4"),
			want: "claude-opus-4",
		},
		{
			name: "returns empty string when not present",
			ctx:  context.Background(),
			want: "",
		},
		{
			name: "returns empty string when wrong type",
			ctx:  context.WithValue(context.Background(), ModelNameContextKey, 789),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetModelNameFromContext(tt.ctx)
			if got != tt.want {
				t.Errorf("GetModelNameFromContext() = %v, want %v", got, tt.want)
			}
		})
	}
}

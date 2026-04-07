package tools

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/history"
	"github.com/charmbracelet/crush/internal/permission"
	"github.com/charmbracelet/crush/internal/pubsub"
)

// --- permission mocks ---

// mockPermissionService always grants permission.
type mockPermissionService struct {
	*pubsub.Broker[permission.PermissionRequest]
}

func (m *mockPermissionService) Request(_ context.Context, _ permission.CreatePermissionRequest) (bool, error) {
	return true, nil
}

func (m *mockPermissionService) Grant(_ permission.PermissionRequest)           {}
func (m *mockPermissionService) Deny(_ permission.PermissionRequest)            {}
func (m *mockPermissionService) GrantPersistent(_ permission.PermissionRequest) {}
func (m *mockPermissionService) AutoApproveSession(_ string)                   {}
func (m *mockPermissionService) SetSkipRequests(_ bool)                        {}
func (m *mockPermissionService) SkipRequests() bool                            { return false }
func (m *mockPermissionService) SubscribeNotifications(_ context.Context) <-chan pubsub.Event[permission.PermissionNotification] {
	return make(<-chan pubsub.Event[permission.PermissionNotification])
}

// denyPermissionService always denies permission.
type denyPermissionService struct {
	*pubsub.Broker[permission.PermissionRequest]
}

func (m *denyPermissionService) Request(_ context.Context, _ permission.CreatePermissionRequest) (bool, error) {
	return false, nil
}

func (m *denyPermissionService) Grant(_ permission.PermissionRequest)           {}
func (m *denyPermissionService) Deny(_ permission.PermissionRequest)            {}
func (m *denyPermissionService) GrantPersistent(_ permission.PermissionRequest) {}
func (m *denyPermissionService) AutoApproveSession(_ string)                   {}
func (m *denyPermissionService) SetSkipRequests(_ bool)                        {}
func (m *denyPermissionService) SkipRequests() bool                            { return false }
func (m *denyPermissionService) SubscribeNotifications(_ context.Context) <-chan pubsub.Event[permission.PermissionNotification] {
	return make(<-chan pubsub.Event[permission.PermissionNotification])
}

// --- history mocks ---

// mockHistoryService is the original simple mock (always succeeds, no tracking).
type mockHistoryService struct {
	*pubsub.Broker[history.File]
}

func (m *mockHistoryService) Create(_ context.Context, _, path, content string) (history.File, error) {
	return history.File{Path: path, Content: content}, nil
}

func (m *mockHistoryService) CreateVersion(_ context.Context, _, _, _ string) (history.File, error) {
	return history.File{}, nil
}

func (m *mockHistoryService) GetByPathAndSession(_ context.Context, path, _ string) (history.File, error) {
	return history.File{Path: path, Content: ""}, nil
}

func (m *mockHistoryService) Get(_ context.Context, _ string) (history.File, error) {
	return history.File{}, nil
}

func (m *mockHistoryService) ListBySession(_ context.Context, _ string) ([]history.File, error) {
	return nil, nil
}

func (m *mockHistoryService) ListLatestSessionFiles(_ context.Context, _ string) ([]history.File, error) {
	return nil, nil
}

func (m *mockHistoryService) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *mockHistoryService) DeleteSessionFiles(_ context.Context, _ string) error {
	return nil
}

// configurableHistoryService allows configuring GetByPathAndSession to return
// errors (simulating "no history row") and tracking which methods are called.
type configurableHistoryService struct {
	*pubsub.Broker[history.File]

	getByPathErr       error               // if non-nil, GetByPathAndSession returns this
	getByPathResult    history.File        // returned when getByPathErr is nil
	createErr          error               // if non-nil, Create returns this
	createVersionErr   error               // if non-nil, CreateVersion returns this
	createCalls        []createCall        // records Create invocations
	createVersionCalls []createVersionCall // records CreateVersion invocations
}

type createCall struct {
	SessionID string
	Path      string
	Content   string
}

type createVersionCall struct {
	SessionID string
	Path      string
	Content   string
}

func newConfigurableHistoryService() *configurableHistoryService {
	return &configurableHistoryService{
		Broker: pubsub.NewBroker[history.File](),
	}
}

func (m *configurableHistoryService) Create(_ context.Context, sessionID, path, content string) (history.File, error) {
	m.createCalls = append(m.createCalls, createCall{sessionID, path, content})
	if m.createErr != nil {
		return history.File{}, m.createErr
	}
	return history.File{Path: path, Content: content}, nil
}

func (m *configurableHistoryService) CreateVersion(_ context.Context, sessionID, path, content string) (history.File, error) {
	m.createVersionCalls = append(m.createVersionCalls, createVersionCall{sessionID, path, content})
	if m.createVersionErr != nil {
		return history.File{}, m.createVersionErr
	}
	return history.File{Path: path, Content: content}, nil
}

func (m *configurableHistoryService) GetByPathAndSession(_ context.Context, _, _ string) (history.File, error) {
	if m.getByPathErr != nil {
		return history.File{}, m.getByPathErr
	}
	return m.getByPathResult, nil
}

func (m *configurableHistoryService) Get(_ context.Context, _ string) (history.File, error) {
	return history.File{}, nil
}

func (m *configurableHistoryService) ListBySession(_ context.Context, _ string) ([]history.File, error) {
	return nil, nil
}

func (m *configurableHistoryService) ListLatestSessionFiles(_ context.Context, _ string) ([]history.File, error) {
	return nil, nil
}

func (m *configurableHistoryService) Delete(_ context.Context, _ string) error {
	return nil
}

func (m *configurableHistoryService) DeleteSessionFiles(_ context.Context, _ string) error {
	return nil
}

// --- filetracker mock ---

// mockFileTrackerService tracks RecordRead calls and returns a configurable LastReadTime.
type mockFileTrackerService struct {
	lastReadTime time.Time
	recorded     []string
}

func (m *mockFileTrackerService) RecordRead(_ context.Context, _, path string) {
	m.recorded = append(m.recorded, path)
}

func (m *mockFileTrackerService) LastReadTime(_ context.Context, _, _ string) time.Time {
	return m.lastReadTime
}

func (m *mockFileTrackerService) ListReadFiles(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}

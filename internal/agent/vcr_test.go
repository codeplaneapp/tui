package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"charm.land/x/vcr"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/cassette"
	"gopkg.in/dnaeon/go-vcr.v4/pkg/recorder"
)

func newAgentRecorder(t *testing.T) *vcr.Recorder {
	t.Helper()

	r, err := recorder.New(
		filepath.Join("testdata", t.Name()),
		recorder.WithMode(recorder.ModeRecordOnce),
		recorder.WithMatcher(agentRequestMatcher()),
		recorder.WithSkipRequestLatency(true),
	)
	if err != nil {
		t.Fatalf("vcr: failed to create recorder: %v", err)
	}

	t.Cleanup(func() {
		if err := r.Stop(); err != nil {
			t.Errorf("vcr: failed to stop recorder: %v", err)
		}
	})

	return r
}

func agentRequestMatcher() recorder.MatcherFunc {
	return func(r *http.Request, i cassette.Request) bool {
		if r.Method != i.Method || r.URL.String() != i.URL {
			return false
		}

		reqModel, err := requestModel(r)
		if err != nil {
			return false
		}
		cassetteModel := requestBodyModel([]byte(i.Body))
		if reqModel == "" || cassetteModel == "" {
			return true
		}
		return reqModel == cassetteModel
	}
}

func requestModel(r *http.Request) (string, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return "", nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	return requestBodyModel(body), nil
}

func requestBodyModel(body []byte) string {
	var payload struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Model
}

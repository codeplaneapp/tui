package agent

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/providers/openaicompat"
	"charm.land/x/vcr"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

type debugCassette struct {
	Interactions []debugInteraction `yaml:"interactions"`
}

type debugInteraction struct {
	Response debugHTTP `yaml:"response"`
}

type debugHTTP struct {
	Body    string              `yaml:"body"`
	Headers map[string][]string `yaml:"headers"`
	Status  string              `yaml:"status"`
	Code    int                 `yaml:"code"`
}

type replayTransport struct {
	t         *testing.T
	idx       int
	reqBodies []string
	resp      []debugInteraction
}

func (r *replayTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.t.Helper()

	body, err := io.ReadAll(req.Body)
	require.NoError(r.t, err)
	r.reqBodies = append(r.reqBodies, string(body))

	require.Less(r.t, r.idx, len(r.resp), "unexpected extra request")
	resp := r.resp[r.idx]
	r.idx++

	return &http.Response{
		Status:     resp.Response.Status,
		StatusCode: resp.Response.Code,
		Header:     http.Header(resp.Response.Headers),
		Body:       io.NopCloser(strings.NewReader(resp.Response.Body)),
		Request:    req,
	}, nil
}

func TestCaptureZAIParallelRequestBodies(t *testing.T) {
	if os.Getenv("CRUSH_DEBUG_CAPTURE_ZAI_REQUESTS") == "" {
		t.Skip("debug only")
	}

	data, err := os.ReadFile("testdata/TestCoderAgent/zai-glm4.6/parallel_tool_calls.yaml")
	require.NoError(t, err)

	var cassette debugCassette
	require.NoError(t, yaml.Unmarshal(data, &cassette))

	rt := &replayTransport{t: t, resp: cassette.Interactions}

	buildModel := func(model string) fantasy.LanguageModel {
		provider, err := openaicompat.New(
			openaicompat.WithBaseURL("https://api.z.ai/api/coding/paas/v4"),
			openaicompat.WithAPIKey("debug"),
			openaicompat.WithHTTPClient(&http.Client{Transport: rt}),
		)
		require.NoError(t, err)

		lm, err := provider.LanguageModel(t.Context(), model)
		require.NoError(t, err)
		return lm
	}

	env := testEnv(t)
	createSimpleGoProject(t, env.workingDir)

	r := vcr.NewRecorder(t)
	agent, err := coderAgent(r, env, buildModel("glm-4.6"), buildModel("glm-4.5-air"))
	require.NoError(t, err)

	session, err := env.sessions.Create(t.Context(), "New Session")
	require.NoError(t, err)

	_, err = agent.Run(t.Context(), SessionAgentCall{
		Prompt:          "use glob to find all .go files and use ls to list the current directory, it is very important that you run both tool calls in parallel",
		SessionID:       session.ID,
		MaxOutputTokens: 10000,
	})
	require.NoError(t, err)

	for i, body := range rt.reqBodies {
		path := filepath.Join("/tmp", "crush-zai-parallel-request-"+strconv.Itoa(i)+".json")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
		t.Logf("wrote %s", path)
	}
}

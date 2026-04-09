package main

import (
	"net/http"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestPprofServerStartsWhenProfileEnabled(t *testing.T) {
	cmd := exec.Command("go", "run", ".")
	cmd.Env = append(os.Environ(), "CODEPLANE_PROFILE=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()

	// Wait for the pprof server to come up.
	var resp *http.Response
	for range 20 {
		time.Sleep(250 * time.Millisecond)
		r, err := http.Get("http://localhost:6060/debug/pprof/")
		if err == nil {
			resp = r
			resp.Body.Close()
			break
		}
	}
	if resp == nil {
		t.Fatal("pprof server never became reachable")
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

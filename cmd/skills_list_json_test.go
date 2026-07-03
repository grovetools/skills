package cmd

import (
	"encoding/json"
	"io"
	"os"
	"testing"
)

// TestSkillsListJSON asserts that `skills list --json` emits a JSON array on
// every code path (including the legacy, non-workspace path), and that each
// element carries a non-empty name. Built-in skills are embedded in the
// binary, so the array is non-empty even outside a workspace.
func TestSkillsListJSON(t *testing.T) {
	// Run from a temp directory so there is no workspace context, forcing the
	// legacy fallback path (skills.go listSkillsLegacy). This is the path that
	// previously ignored --json and printed the 2-column table.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmp := t.TempDir()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	// Capture os.Stdout, which the command writes JSON to directly.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = origStdout })

	cmd := newSkillsListCmd()
	cmd.SetArgs([]string{"--json"})
	runErr := cmd.Execute()

	_ = w.Close()
	os.Stdout = origStdout
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	if runErr != nil {
		t.Fatalf("list --json returned error: %v\noutput: %s", runErr, out)
	}

	var arr []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("output is not a JSON array: %v\noutput: %s", err, out)
	}
	if len(arr) == 0 {
		t.Fatalf("expected at least one skill (built-ins are embedded), got empty array\noutput: %s", out)
	}
	for i, el := range arr {
		if el.Name == "" {
			t.Errorf("element %d has empty name\noutput: %s", i, out)
		}
	}
}

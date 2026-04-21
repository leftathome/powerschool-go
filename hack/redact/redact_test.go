package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedact_PlainSubstitution(t *testing.T) {
	in := `<html>Hello John Smith, your UID is 1099999099.</html>`
	c := &Config{Replacements: map[string]string{
		"John Smith": "Student Alpha",
		"1099999099":  "UID-1001",
	}}
	got, err := Redact(in, c)
	if err != nil {
		t.Fatalf("Redact error: %v", err)
	}
	want := `<html>Hello Student Alpha, your UID is UID-1001.</html>`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedact_LongestFirst(t *testing.T) {
	// "Student Alpha" must beat "Student" so the longer key wins.
	in := `Student Alpha and Student Beta.`
	c := &Config{Replacements: map[string]string{
		"Student Alpha": "AAA",
		"Student":       "BBB",
		"Student Beta":  "CCC",
	}}
	got, err := Redact(in, c)
	if err != nil {
		t.Fatalf("Redact error: %v", err)
	}
	want := `AAA and CCC.`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRedact_Idempotent(t *testing.T) {
	in := `<a href="/parent/grading_report/1099999099">Jane Q. Smith</a>`
	c := &Config{Replacements: map[string]string{
		"1099999099":     "UID-1001",
		"Jane Q. Smith": "Student Alpha",
	}}
	once, err := Redact(in, c)
	if err != nil {
		t.Fatalf("first pass: %v", err)
	}
	twice, err := Redact(once, c)
	if err != nil {
		t.Fatalf("second pass: %v", err)
	}
	if once != twice {
		t.Errorf("not idempotent:\n once: %q\n twice: %q", once, twice)
	}
}

func TestRedact_Deterministic(t *testing.T) {
	in := `alpha beta gamma alpha beta`
	c := &Config{Replacements: map[string]string{
		"alpha": "A",
		"beta":  "B",
		"gamma": "G",
	}}
	first, err := Redact(in, c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		got, err := Redact(in, c)
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatalf("non-deterministic output: %q vs %q", first, got)
		}
	}
}

func TestRedact_RejectsReplacementContainingKey(t *testing.T) {
	// "Alpha" is a find key AND appears inside the replacement for
	// "Student Alpha". Running this would mean a second pass rewrites
	// "Alpha" inside "Student Alpha-placeholder". Refuse.
	c := &Config{Replacements: map[string]string{
		"Student Alpha": "Student Alpha-placeholder",
		"Alpha":         "X",
	}}
	_, err := Redact("irrelevant", c)
	if err == nil {
		t.Fatal("expected idempotence-guard error, got nil")
	}
	if !strings.Contains(err.Error(), "idempotence") {
		t.Errorf("error = %v, want it to mention idempotence", err)
	}
}

func TestRedact_RejectsEmptyKey(t *testing.T) {
	c := &Config{Replacements: map[string]string{"": "X"}}
	if _, err := Redact("x", c); err == nil {
		t.Error("expected error for empty key, got nil")
	}
}

func TestRedact_RejectsEmptyValue(t *testing.T) {
	c := &Config{Replacements: map[string]string{"X": ""}}
	if _, err := Redact("X", c); err == nil {
		t.Error("expected error for empty value, got nil")
	}
}

func TestRedact_NilConfig(t *testing.T) {
	got, err := Redact("untouched", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "untouched" {
		t.Errorf("got %q, want 'untouched'", got)
	}
}

func TestRedact_EmptyConfig(t *testing.T) {
	got, err := Redact("untouched", &Config{})
	if err != nil {
		t.Fatal(err)
	}
	if got != "untouched" {
		t.Errorf("got %q, want 'untouched'", got)
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	content := `{"replacements":{"alpha":"A","beta":"B"}}`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c.Replacements["alpha"] != "A" || c.Replacements["beta"] != "B" {
		t.Errorf("replacements = %+v", c.Replacements)
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(path); err == nil {
		t.Error("expected parse error, got nil")
	}
}

func TestRun_MissingConfig(t *testing.T) {
	err := run("/tmp/ignored", "/tmp/ignored", "/tmp/does-not-exist-xyzzy.json")
	if err == nil {
		t.Error("expected error for missing config, got nil")
	}
}

func TestRun_MissingInput(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	if err := os.WriteFile(cfgPath, []byte(`{"replacements":{"a":"b"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := run(filepath.Join(dir, "missing.html"), filepath.Join(dir, "out.html"), cfgPath)
	if err == nil {
		t.Error("expected error for missing input, got nil")
	}
}

func TestRun_InvalidReplacement(t *testing.T) {
	// Config has an idempotence violation (replacement contains find
	// key); Redact() refuses and run() should propagate.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	inPath := filepath.Join(dir, "in.html")
	outPath := filepath.Join(dir, "out.html")
	if err := os.WriteFile(cfgPath, []byte(`{"replacements":{"Student Alpha":"Student Alpha-ph","Alpha":"X"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, []byte("any"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(inPath, outPath, cfgPath); err == nil {
		t.Error("expected idempotence-guard error, got nil")
	}
}

func TestRun_WriteFailure(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	inPath := filepath.Join(dir, "in.html")
	if err := os.WriteFile(cfgPath, []byte(`{"replacements":{"a":"b"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	// outPath points to a directory — WriteFile will fail.
	err := run(inPath, dir, cfgPath)
	if err == nil {
		t.Error("expected write error, got nil")
	}
}

// TestMain_CLI builds the tool and runs it end-to-end. Covers main()
// itself (flag parsing, exit codes, arg validation).
func TestMain_CLI(t *testing.T) {
	if testing.Short() {
		t.Skip("short mode skips subprocess test")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "redact.test-bin")
	if cmd := exec.Command("go", "build", "-o", bin, "."); true {
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}

	cfgPath := filepath.Join(dir, "cfg.json")
	inPath := filepath.Join(dir, "in.html")
	outPath := filepath.Join(dir, "out.html")
	if err := os.WriteFile(cfgPath, []byte(`{"replacements":{"Real":"Placeholder"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, []byte("<p>Real name</p>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Happy path.
	cmd := exec.Command(bin, "-in", inPath, "-out", outPath, "-config", cfgPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<p>Placeholder name</p>" {
		t.Errorf("out = %q", got)
	}

	// Missing args → exit 2.
	cmd = exec.Command(bin)
	out, err = cmd.CombinedOutput()
	if err == nil {
		t.Errorf("expected non-zero exit for missing args, got success\n%s", out)
	}
	if !strings.Contains(string(out), "usage:") {
		t.Errorf("stderr missing usage hint: %s", out)
	}
}

func TestRun_EndToEnd(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	inPath := filepath.Join(dir, "in.html")
	outPath := filepath.Join(dir, "out.html")

	if err := os.WriteFile(cfgPath, []byte(`{"replacements":{"John Smith":"Student Alpha"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(inPath, []byte(`<p>John Smith</p>`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run(inPath, outPath, cfgPath); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `<p>Student Alpha</p>` {
		t.Errorf("out = %q", got)
	}
}

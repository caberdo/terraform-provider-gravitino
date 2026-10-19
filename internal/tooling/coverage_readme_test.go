package tooling_test

// These tests drive scripts/coverage-readme.sh as CI does: the generated badge
// and coverage block in README.md must come from .github/coverage-baseline.txt,
// the marker guard must be as strict as the splicer, and a configuration or
// marker error must never rewrite (or truncate) the README.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const scriptRel = "../../scripts/coverage-readme.sh"

const baselineFixture = `# statement-coverage baseline
measured_internal_client=77.2
measured_internal_models=84.7
measured_internal_provider=70.3
measured_total=79.2
measured_commit=9c94a46
measured_date=2026-10-19
floor_internal_client=76.5
floor_internal_models=84.0
floor_internal_provider=69.5
floor_total=78.5
target=80
gold=90
`

const readmeFixture = `# Provider

[![CI](x)](#)
<!-- coverage-badge:start -->
stale badge
<!-- coverage-badge:end -->
Intro paragraph.

<!-- coverage:start -->
stale block
<!-- coverage:end -->

Tail paragraph.
`

var markers = []string{
	"<!-- coverage-badge:start -->",
	"<!-- coverage-badge:end -->",
	"<!-- coverage:start -->",
	"<!-- coverage:end -->",
}

type fixture struct {
	readme   string
	baseline string
}

func newFixture(t *testing.T, readme, baseline string) fixture {
	t.Helper()
	dir := t.TempDir()
	f := fixture{
		readme:   filepath.Join(dir, "README.md"),
		baseline: filepath.Join(dir, "coverage-baseline.txt"),
	}
	if readme != "" {
		mustWrite(t, f.readme, readme)
	}
	if baseline != "" {
		mustWrite(t, f.baseline, baseline)
	}
	return f
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(data)
}

func scriptPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(scriptRel)
	if err != nil {
		t.Fatalf("resolving %s: %v", scriptRel, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("coverage-readme.sh not found at %s: %v", path, err)
	}
	return path
}

func (f fixture) run(t *testing.T, args ...string) (int, string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "bash", append([]string{scriptPath(t), f.readme, f.baseline}, args...)...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("running coverage-readme.sh: %v\n%s", err, out)
	}
	return exit.ExitCode(), string(out)
}

func countLine(t *testing.T, content, line string) int {
	t.Helper()
	count := 0
	for _, got := range strings.Split(content, "\n") {
		if got == line {
			count++
		}
	}
	return count
}

func TestCoverageReadmeGeneratesTheBadgeAndBlock(t *testing.T) {
	f := newFixture(t, readmeFixture, baselineFixture)

	code, out := f.run(t)
	if code != 0 {
		t.Fatalf("write mode exited %d, want 0\n%s", code, out)
	}

	got := mustRead(t, f.readme)
	for _, want := range []string{
		"[![Coverage](https://img.shields.io/badge/coverage-79.2%25-yellow)](#coverage)",
		"internal/client",
		"77.2%   floor 76.5%",
		"internal/models",
		"84.7%   floor 84.0%",
		"internal/provider",
		"70.3%   floor 69.5%",
		"total",
		"-> 80%, then 90%",
		"`-covermode=atomic -coverpkg=./internal/...`",
		"`make coverage-merge`",
		"Measured on `9c94a46` (unit + acceptance) on 2026-10-19.",
		"[![CI](x)](#)",
		"Tail paragraph.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated README does not contain %q\n%s", want, got)
		}
	}
	if strings.Contains(got, "stale") {
		t.Errorf("generated README still carries the stale placeholder content:\n%s", got)
	}
	for _, marker := range markers {
		if n := countLine(t, got, marker); n != 1 {
			t.Errorf("generated README has %d copies of %q, want exactly 1", n, marker)
		}
	}

	// The baseline order is kept, with `total` last.
	order := []string{"internal/client", "internal/models", "internal/provider", "total"}
	last := -1
	for _, scope := range order {
		i := strings.Index(got, scope)
		if i < 0 {
			t.Fatalf("generated README does not render scope %q", scope)
		}
		if i < last {
			t.Errorf("scope %q is out of baseline order", scope)
		}
		last = i
	}

	code, out = f.run(t, "--check")
	if code != 0 {
		t.Fatalf("--check on a freshly generated README exited %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "in sync") {
		t.Errorf("--check did not report the README in sync: %s", out)
	}
}

func TestCoverageReadmeCheckFailsOnDrift(t *testing.T) {
	f := newFixture(t, readmeFixture, baselineFixture)
	if code, out := f.run(t); code != 0 {
		t.Fatalf("write mode exited %d, want 0\n%s", code, out)
	}

	drifted := strings.Replace(baselineFixture, "measured_total=79.2", "measured_total=80.4", 1)
	mustWrite(t, f.baseline, drifted)
	before := mustRead(t, f.readme)

	code, out := f.run(t, "--check")
	if code != 1 {
		t.Fatalf("--check with a drifted baseline exited %d, want 1\n%s", code, out)
	}
	if !strings.Contains(out, "out of date") {
		t.Errorf("--check did not say the README is out of date: %s", out)
	}
	if !strings.Contains(out, "80.4") {
		t.Errorf("--check diff does not show the new measurement: %s", out)
	}
	if after := mustRead(t, f.readme); after != before {
		t.Errorf("--check modified the README:\n--- before\n%s\n--- after\n%s", before, after)
	}
}

func TestCoverageReadmeRejectsBrokenMarkersWithoutWriting(t *testing.T) {
	cases := []struct {
		name   string
		readme string
	}{
		{
			name:   "trailing whitespace on a marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage:start -->", "<!-- coverage:start --> ", 1),
		},
		{
			name:   "indented marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage:start -->", "  <!-- coverage:start -->", 1),
		},
		{
			name:   "CRLF line endings",
			readme: strings.ReplaceAll(readmeFixture, "\n", "\r\n"),
		},
		{
			name:   "missing coverage end marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage:end -->\n", "", 1),
		},
		{
			name:   "missing badge end marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage-badge:end -->\n", "", 1),
		},
		{
			name:   "missing coverage start marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage:start -->\n", "", 1),
		},
		{
			name: "duplicated coverage start marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage:start -->",
				"<!-- coverage:start -->\n<!-- coverage:start -->", 1),
		},
		{
			name: "duplicated badge end marker",
			readme: strings.Replace(readmeFixture, "<!-- coverage-badge:end -->",
				"<!-- coverage-badge:end -->\n<!-- coverage-badge:end -->", 1),
		},
		{
			name: "coverage end before start",
			readme: strings.NewReplacer(
				"<!-- coverage:start -->", "<!-- coverage:end -->",
				"<!-- coverage:end -->", "<!-- coverage:start -->",
			).Replace(readmeFixture),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, tc.readme, baselineFixture)
			before := mustRead(t, f.readme)

			code, out := f.run(t)
			if code != 2 {
				t.Fatalf("write mode exited %d, want 2 (configuration error)\n%s", code, out)
			}
			if !strings.Contains(out, "marker") {
				t.Errorf("write mode did not report the marker problem: %s", out)
			}
			if after := mustRead(t, f.readme); after != before {
				t.Errorf("write mode modified the README despite the marker error:\n%s", after)
			}

			code, out = f.run(t, "--check")
			if code != 2 {
				t.Fatalf("--check exited %d, want 2 (a marker error is not drift)\n%s", code, out)
			}
			if after := mustRead(t, f.readme); after != before {
				t.Errorf("--check modified the README despite the marker error:\n%s", after)
			}
		})
	}
}

func TestCoverageReadmeScopesComeFromTheBaseline(t *testing.T) {
	withWidget := baselineFixture +
		"measured_internal_widget=42.0\nfloor_internal_widget=41.5\n"

	f := newFixture(t, readmeFixture, withWidget)
	if code, out := f.run(t); code != 0 {
		t.Fatalf("write mode exited %d, want 0\n%s", code, out)
	}
	got := mustRead(t, f.readme)
	if !strings.Contains(got, "internal/widget") {
		t.Errorf("baseline scope internal/widget is missing from the block:\n%s", got)
	}
	if !strings.Contains(got, "42.0%   floor 41.5%") {
		t.Errorf("baseline scope internal/widget did not render its numbers:\n%s", got)
	}
	if strings.Index(got, "internal/widget") > strings.Index(got, "total") {
		t.Errorf("total is not the last row:\n%s", got)
	}

	t.Run("measured scope without a floor", func(t *testing.T) {
		f := newFixture(t, readmeFixture, withWidget)
		mustWrite(t, f.baseline, strings.Replace(withWidget, "floor_internal_widget=41.5\n", "", 1))
		before := mustRead(t, f.readme)
		code, out := f.run(t)
		if code != 2 {
			t.Fatalf("exited %d, want 2\n%s", code, out)
		}
		if !strings.Contains(out, "no floor_internal_widget") {
			t.Errorf("error does not name the missing floor: %s", out)
		}
		if mustRead(t, f.readme) != before {
			t.Error("README was rewritten despite the configuration error")
		}
	})

	t.Run("floor without a measured scope", func(t *testing.T) {
		f := newFixture(t, readmeFixture, baselineFixture+"floor_internal_ghost=1.0\n")
		before := mustRead(t, f.readme)
		code, out := f.run(t)
		if code != 2 {
			t.Fatalf("exited %d, want 2\n%s", code, out)
		}
		if !strings.Contains(out, "no measured_internal_ghost") {
			t.Errorf("error does not name the orphaned floor: %s", out)
		}
		if mustRead(t, f.readme) != before {
			t.Error("README was rewritten despite the configuration error")
		}
	})

	t.Run("no total scope", func(t *testing.T) {
		f := newFixture(t, readmeFixture, strings.Replace(baselineFixture, "measured_total=79.2\n", "", 1))
		if code, out := f.run(t); code != 2 || !strings.Contains(out, "measured_total") {
			t.Fatalf("exited %d, want 2 naming measured_total\n%s", code, out)
		}
	})

	t.Run("missing measurement metadata", func(t *testing.T) {
		f := newFixture(t, readmeFixture, strings.Replace(baselineFixture, "measured_commit=9c94a46\n", "", 1))
		if code, out := f.run(t); code != 2 || !strings.Contains(out, "measured_commit") {
			t.Fatalf("exited %d, want 2 naming measured_commit\n%s", code, out)
		}
	})

	t.Run("unknown key", func(t *testing.T) {
		f := newFixture(t, readmeFixture, baselineFixture+"measure_total=79.2\n")
		if code, out := f.run(t); code != 2 || !strings.Contains(out, "unknown key") {
			t.Fatalf("exited %d, want 2 reporting an unknown key\n%s", code, out)
		}
	})

	t.Run("missing baseline", func(t *testing.T) {
		dir := t.TempDir()
		f := fixture{
			readme:   filepath.Join(dir, "README.md"),
			baseline: filepath.Join(dir, "absent.txt"),
		}
		mustWrite(t, f.readme, readmeFixture)
		if code, out := f.run(t); code != 2 || !strings.Contains(out, "not found") {
			t.Fatalf("exited %d, want 2 reporting a missing baseline\n%s", code, out)
		}
	})
}

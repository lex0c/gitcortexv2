package report

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lex0c/gitcortex/internal/stats"
)

const fixtureJSONL = `{"type":"commit","sha":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","author_name":"Alice","author_email":"alice@test.com","author_date":"2024-01-15T10:00:00Z","additions":40,"deletions":5,"files_changed":2}
{"type":"commit_file","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","path_current":"main.go","status":"M","additions":30,"deletions":5}
{"type":"commit_file","commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","path_current":"util.go","status":"M","additions":10,"deletions":0}
{"type":"commit","sha":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","author_name":"Bob","author_email":"bob@test.com","author_date":"2024-02-20T15:30:00Z","additions":20,"deletions":10,"files_changed":1}
{"type":"commit_file","commit":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","path_current":"main.go","status":"M","additions":20,"deletions":10}
{"type":"commit","sha":"cccccccccccccccccccccccccccccccccccccccc","author_name":"Alice","author_email":"alice@test.com","author_date":"2024-03-05T09:00:00Z","additions":30,"deletions":10,"files_changed":2}
{"type":"commit_file","commit":"cccccccccccccccccccccccccccccccccccccccc","path_current":"main.go","status":"M","additions":20,"deletions":5}
{"type":"commit_file","commit":"cccccccccccccccccccccccccccccccccccccccc","path_current":"util.go","status":"M","additions":10,"deletions":5}
{"type":"commit","sha":"dddddddddddddddddddddddddddddddddddddddd","author_name":"Alice","author_email":"alice@test.com","author_date":"2024-03-15T14:00:00Z","additions":10,"deletions":5,"files_changed":1}
{"type":"commit_file","commit":"dddddddddddddddddddddddddddddddddddddddd","path_current":"readme.md","status":"M","additions":10,"deletions":5}
`

func writeFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture.jsonl")
	if err := os.WriteFile(path, []byte(fixtureJSONL), 0644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func loadFixture(t *testing.T) *stats.Dataset {
	t.Helper()
	ds, err := stats.LoadJSONL(writeFixture(t))
	if err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	return ds
}

func TestGenerate_SmokeRender(t *testing.T) {
	ds := loadFixture(t)
	var buf bytes.Buffer
	err := Generate(&buf, ds, "testrepo", 10, stats.StatsFlags{CouplingMinChanges: 1, NetworkMinFiles: 1})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := buf.String()

	wants := []string{
		"<!DOCTYPE html>",
		"testrepo",
		`id="act-heatmap"`,
		`id="act-table"`,
		`getElementById('act-heatmap')`,
		`getElementById('act-table')`,
		"<th>Del/Add</th>",
		"Concentration",
		"Top Contributors",
		"Developer Profiles",
		`href="https://github.com/lex0c/gitcortex"`,
		`target="_blank"`,
		`rel="noopener noreferrer"`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in output", w)
		}
	}

	if strings.Contains(out, "<no value>") {
		t.Errorf("output contains `<no value>` — template rendered a nil field")
	}
}

func TestGenerate_EmptyDataset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ds, err := stats.LoadJSONL(path)
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}

	var buf bytes.Buffer
	if err := Generate(&buf, ds, "empty", 10, stats.StatsFlags{}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	out := buf.String()

	// Empty dataset should render the "no data" path, not fall through to
	// "extremely concentrated" (pre-fix bug: FilesPct80Churn=0.0 was <= 10).
	wantCount := strings.Count(out, "no data")
	if wantCount < 3 {
		t.Errorf("expected at least 3 'no data' markers (Files/Devs/Dirs), got %d", wantCount)
	}
	if strings.Contains(out, "extremely concentrated") {
		t.Errorf("output should not claim 'extremely concentrated' for empty dataset")
	}
	if !strings.Contains(out, "⚪") {
		t.Errorf("output should contain neutral emoji ⚪ for empty cards")
	}
	if strings.Contains(out, "<no value>") {
		t.Errorf("template rendered nil field as <no value>")
	}
}

func TestGenerateProfile_SmokeRender(t *testing.T) {
	ds := loadFixture(t)
	var buf bytes.Buffer
	err := GenerateProfile(&buf, ds, "testrepo", "alice@test.com")
	if err != nil {
		t.Fatalf("GenerateProfile: %v", err)
	}
	out := buf.String()

	wants := []string{
		"<!DOCTYPE html>",
		"Alice",
		"alice@test.com",
		`id="prof-act-heatmap"`,
		`id="prof-act-table"`,
		`getElementById('prof-act-heatmap')`,
		`getElementById('prof-act-table')`,
		"<th>Del/Add</th>",
		"Scope",
		"Top Files",
		`target="_blank"`,
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in output", w)
		}
	}

	if strings.Contains(out, "<no value>") {
		t.Errorf("output contains `<no value>` — template rendered a nil field")
	}
}

func TestGenerateProfile_UnknownEmail(t *testing.T) {
	ds := loadFixture(t)
	var buf bytes.Buffer
	err := GenerateProfile(&buf, ds, "testrepo", "ghost@nowhere.com")
	if err == nil {
		t.Fatal("expected error for unknown email, got nil")
	}
}

func TestBuildActivityGrid_Monthly(t *testing.T) {
	raw := []stats.ActivityBucket{
		{Period: "2024-01", Commits: 5, Additions: 100, Deletions: 20},
		{Period: "2024-03", Commits: 2, Additions: 50, Deletions: 10},
		{Period: "2025-07", Commits: 1, Additions: 10, Deletions: 0},
	}
	years, grid, max := buildActivityGrid(raw)

	if len(years) != 2 || years[0] != "2024" || years[1] != "2025" {
		t.Errorf("years = %v, want [2024 2025]", years)
	}
	if len(grid) != 2 {
		t.Fatalf("grid rows = %d, want 2", len(grid))
	}
	if len(grid[0]) != 12 {
		t.Errorf("grid cols = %d, want 12", len(grid[0]))
	}
	if !grid[0][0].HasData || grid[0][0].Commits != 5 {
		t.Errorf("grid[2024][Jan] = %+v, want Commits=5 HasData=true", grid[0][0])
	}
	if grid[0][1].HasData {
		t.Errorf("grid[2024][Feb] should be empty, got %+v", grid[0][1])
	}
	if !grid[0][2].HasData || grid[0][2].Commits != 2 {
		t.Errorf("grid[2024][Mar] = %+v, want Commits=2", grid[0][2])
	}
	if !grid[1][6].HasData || grid[1][6].Commits != 1 {
		t.Errorf("grid[2025][Jul] = %+v, want Commits=1", grid[1][6])
	}
	if max != 5 {
		t.Errorf("maxCommits = %d, want 5", max)
	}
}

func TestComputePareto(t *testing.T) {
	ds := loadFixture(t)
	p := ComputePareto(ds)

	// Fixture has 2 devs (Alice: 3 commits, Bob: 1), 3 files, all in root dir.
	if p.TotalDevs != 2 {
		t.Errorf("TotalDevs = %d, want 2", p.TotalDevs)
	}
	if p.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", p.TotalFiles)
	}

	// Alice = 3/4 = 75% of commits (< 80%), so 80% threshold needs both devs.
	if p.TopCommitDevs != 2 {
		t.Errorf("TopCommitDevs = %d, want 2 (both devs needed for 80%%)", p.TopCommitDevs)
	}
	if p.DevsPct80Commits != 100.0 {
		t.Errorf("DevsPct80Commits = %.1f, want 100.0", p.DevsPct80Commits)
	}

	// File churn: main.go=90, util.go=25, readme.md=15 → total=130, 80%=104.
	// Cumulative: main=90 (<104) → util 115 (≥104) → stop. Top=2 of 3.
	if p.TopChurnFiles != 2 {
		t.Errorf("TopChurnFiles = %d, want 2", p.TopChurnFiles)
	}

	// Percentages must be in [0, 100].
	for _, v := range []float64{p.FilesPct80Churn, p.DevsPct80Commits, p.DevsPct80Churn, p.DirsPct80Churn} {
		if v < 0 || v > 100 {
			t.Errorf("pct out of range: %.1f", v)
		}
	}

	// DevsPct80Churn should also be populated (fixture has non-zero churn).
	if p.TopChurnDevs == 0 {
		t.Errorf("TopChurnDevs = 0, want > 0 (devs have non-zero churn in fixture)")
	}
}

// Documents current behavior: buildActivityGrid only parses "YYYY-MM".
// Daily ("YYYY-MM-DD") periods collapse multiple days into a single month cell.
// Weekly/yearly formats fail the Sscanf and are dropped.
// The HTML activity heatmap is monthly by design.
func TestBuildActivityGrid_NonMonthlyCollapsesOrDrops(t *testing.T) {
	daily := []stats.ActivityBucket{
		{Period: "2024-01-15", Commits: 3},
		{Period: "2024-01-20", Commits: 4},
	}
	_, grid, _ := buildActivityGrid(daily)
	if len(grid) != 1 {
		t.Fatalf("expected 1 year row, got %d", len(grid))
	}
	// Both days collapse into January (month 0). Only the last one written wins.
	if !grid[0][0].HasData {
		t.Errorf("January should have data (collapsed from daily buckets)")
	}

	weekly := []stats.ActivityBucket{
		{Period: "2024-W03", Commits: 3},
	}
	years, _, _ := buildActivityGrid(weekly)
	if len(years) != 0 {
		t.Errorf("weekly periods should be dropped (invalid month parse), got years=%v", years)
	}
}

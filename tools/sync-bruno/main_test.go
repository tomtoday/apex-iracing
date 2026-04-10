package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRoot returns the absolute path to the repository root, two levels up from
// the tools/sync-bruno package directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("cannot determine repo root: %v", err)
	}
	return root
}

// loadDocsOrSkip reads data/data-docs.json from the repo root, skipping the
// test if the file is absent (it requires prior authentication via the web app).
func loadDocsOrSkip(t *testing.T) map[string]map[string]endpointDoc {
	t.Helper()
	path := filepath.Join(repoRoot(t), "data", "data-docs.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("data-docs.json not present — authenticate via the web app first (%v)", err)
	}
	var docs map[string]map[string]endpointDoc
	if err := json.Unmarshal(raw, &docs); err != nil {
		t.Fatalf("invalid data-docs.json: %v", err)
	}
	return docs
}

// ── Unit tests ────────────────────────────────────────────────────────────────

func TestTitleCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"get", "Get"},
		{"oval", "Oval"},
		{"world_records", "World_Records"},
		{"sports_car", "Sports_Car"},
		{"cust_league_sessions", "Cust_League_Sessions"},
		{"season_tt_standings", "Season_Tt_Standings"},
		{"member_season_results", "Member_Season_Results"},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := titleCase(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestBruFilename(t *testing.T) {
	cases := []struct{ prefix, endpoint, want string }{
		{"Stats", "world_records", "Data-Stats-World_Records.bru"},
		{"League", "get", "Data-League-Get.bru"},
		{"DriverStats", "sports_car", "Data-DriverStats-Sports_Car.bru"},
		{"Car", "assets", "Data-Car-Assets.bru"},
		{"TimeAttack", "member_season_results", "Data-TimeAttack-Member_Season_Results.bru"},
	}
	for _, c := range cases {
		t.Run(c.endpoint, func(t *testing.T) {
			if got := bruFilename(c.prefix, c.endpoint); got != c.want {
				t.Errorf("bruFilename(%q, %q) = %q, want %q", c.prefix, c.endpoint, got, c.want)
			}
		})
	}
}

func TestNoteStrings(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		if got := noteStrings(nil); len(got) != 0 {
			t.Errorf("want empty, got %v", got)
		}
	})

	t.Run("string", func(t *testing.T) {
		raw := json.RawMessage(`"Always the authenticated member."`)
		got := noteStrings(raw)
		if len(got) != 1 || got[0] != "Always the authenticated member." {
			t.Errorf("got %v", got)
		}
	})

	t.Run("array", func(t *testing.T) {
		raw := json.RawMessage(`["first note", "second note"]`)
		got := noteStrings(raw)
		if len(got) != 2 || got[0] != "first note" || got[1] != "second note" {
			t.Errorf("got %v", got)
		}
	})
}

func TestMaxSeqInFolder(t *testing.T) {
	dir := t.TempDir()

	t.Run("empty folder", func(t *testing.T) {
		if got := maxSeqInFolder(dir); got != 0 {
			t.Errorf("want 0, got %d", got)
		}
	})

	for _, seq := range []int{3, 12, 1} {
		f, err := os.CreateTemp(dir, "*.bru")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(f, "meta {\n  seq: %d\n}\n", seq)
		f.Close()
	}

	t.Run("returns max", func(t *testing.T) {
		if got := maxSeqInFolder(dir); got != 12 {
			t.Errorf("want 12, got %d", got)
		}
	})
}

func TestGenerateBRU_WithParams(t *testing.T) {
	dir := t.TempDir()
	statsDir := filepath.Join(dir, "05-Stats")
	os.MkdirAll(statsDir, 0o755)
	// Seed an existing file so seq detection increments correctly.
	os.WriteFile(filepath.Join(statsDir, "seed.bru"), []byte("meta {\n  seq: 4\n}\n"), 0o644)

	doc := endpointDoc{
		Parameters: map[string]parameterDoc{
			"car_id":      {Type: "number", Required: true},
			"track_id":    {Type: "number", Required: true},
			"season_year": {Type: "number", Note: "Limit to a given year."},
		},
	}
	if err := generateBRU(dir, "stats", "world_records", doc); err != nil {
		t.Fatalf("generateBRU: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(statsDir, "Data-Stats-World_Records.bru"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	s := string(content)

	for _, want := range []string{
		"name: Data-Stats-World_Records",
		"seq: 5", // max(4) + 1
		"url: {{base_url}}/data/stats/world_records",
		"auth: inherit",
		"params:query {",
		"  car_id: ",
		"  track_id: ",
		"  season_year: ",
		"docs {",
		"| car_id | number | yes |",
		"| track_id | number | yes |",
		"| season_year | number | no | Limit to a given year. |",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q\n\nFull content:\n%s", want, s)
		}
	}
}

func TestGenerateBRU_RequiredParamsFirst(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "08-Leagues"), 0o755)

	doc := endpointDoc{
		Parameters: map[string]parameterDoc{
			"retired":   {Type: "boolean"},
			"league_id": {Type: "number", Required: true},
			"season_id": {Type: "number", Required: true},
		},
	}
	if err := generateBRU(dir, "league", "seasons", doc); err != nil {
		t.Fatalf("generateBRU: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "08-Leagues", "Data-League-Seasons.bru"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	s := string(content)

	leaguePos := strings.Index(s, "league_id:")
	seasonPos := strings.Index(s, "season_id:")
	retiredPos := strings.Index(s, "retired:")

	if leaguePos < 0 || seasonPos < 0 || retiredPos < 0 {
		t.Fatal("missing expected params")
	}
	if retiredPos < leaguePos || retiredPos < seasonPos {
		t.Error("optional param 'retired' appears before required params in params:query")
	}
}

func TestGenerateBRU_NoteNoParams(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "02-Member"), 0o755)

	doc := endpointDoc{
		Note: json.RawMessage(`"Always the authenticated member."`),
	}
	if err := generateBRU(dir, "member", "info", doc); err != nil {
		t.Fatalf("generateBRU: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "02-Member", "Data-Member-Info.bru"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	s := string(content)

	if strings.Contains(s, "params:query") {
		t.Error("no-param endpoint should not have params:query block")
	}
	if !strings.Contains(s, "docs {") {
		t.Error("endpoint with a note should have a docs block")
	}
	if !strings.Contains(s, "Always the authenticated member.") {
		t.Error("docs block should contain the note text")
	}
}

func TestGenerateBRU_NoAnnotations(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "04-Tracks"), 0o755)

	if err := generateBRU(dir, "track", "get", endpointDoc{}); err != nil {
		t.Fatalf("generateBRU: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "04-Tracks", "Data-Track-Get.bru"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	s := string(content)

	if strings.Contains(s, "params:query") {
		t.Error("no-param endpoint should not have params:query block")
	}
	if strings.Contains(s, "docs {") {
		t.Error("endpoint with no note or params should not have docs block")
	}
}

func TestGenerateBRU_ArrayNote(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "03-Cars"), 0o755)

	doc := endpointDoc{
		Note: json.RawMessage(`["image paths are relative to https://images-static.iracing.com/", "second note"]`),
	}
	if err := generateBRU(dir, "car", "assets", doc); err != nil {
		t.Fatalf("generateBRU: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "03-Cars", "Data-Car-Assets.bru"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	s := string(content)

	if !strings.Contains(s, "image paths are relative to") {
		t.Error("docs block should contain first array note")
	}
	if !strings.Contains(s, "second note") {
		t.Error("docs block should contain second array note")
	}
}

// ── Integration tests: Bruno collection quality ───────────────────────────────
//
// These tests require data/data-docs.json, which is generated by authenticating
// through the web app. They skip gracefully when that file is absent so CI
// without credentials still passes the unit tests above.
//
// To run all tests including integration:
//
//	cd tools && go test ./sync-bruno/

func TestCollectionComplete(t *testing.T) {
	docs := loadDocsOrSkip(t)
	brunoURLs, err := scanBrunoURLs(filepath.Join(repoRoot(t), "bruno"))
	if err != nil {
		t.Fatalf("scan Bruno: %v", err)
	}

	for ns, endpoints := range docs {
		if _, known := categories[ns]; !known {
			t.Errorf("unknown namespace %q: add to categories map in sync-bruno/main.go", ns)
			continue
		}
		for ep := range endpoints {
			ep := ep // capture
			t.Run(ns+"/"+ep, func(t *testing.T) {
				t.Parallel()
				path := fmt.Sprintf("/data/%s/%s", ns, ep)
				if !brunoURLs[path] {
					t.Errorf("no Bruno file found for %s", path)
				}
			})
		}
	}
}

func TestCollectionNoStaleFiles(t *testing.T) {
	docs := loadDocsOrSkip(t)
	brunoURLs, err := scanBrunoURLs(filepath.Join(repoRoot(t), "bruno"))
	if err != nil {
		t.Fatalf("scan Bruno: %v", err)
	}

	docURLs := buildDocURLs(docs)
	for url := range brunoURLs {
		url := url // capture
		t.Run(url, func(t *testing.T) {
			t.Parallel()
			if !docURLs[url] {
				t.Errorf("Bruno file for %s not found in iRacing API docs — endpoint may have been removed", url)
			}
		})
	}
}

func TestBrunoParamsMatch(t *testing.T) {
	docs := loadDocsOrSkip(t)
	brunoRoot := filepath.Join(repoRoot(t), "bruno")

	// Build a URL → doc map for quick lookup.
	docsByURL := make(map[string]endpointDoc, 100)
	for ns, endpoints := range docs {
		for ep, doc := range endpoints {
			docsByURL[fmt.Sprintf("/data/%s/%s", ns, ep)] = doc
		}
	}

	err := filepath.Walk(brunoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".bru" {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		m := urlRe.FindSubmatch(content)
		if m == nil {
			return nil
		}
		urlPath := string(m[1])
		doc, ok := docsByURL[urlPath]
		if !ok || len(doc.Parameters) == 0 {
			return nil // no parameters to check
		}

		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			s := string(content)

			if !strings.Contains(s, "params:query {") {
				t.Errorf("%s: API docs define %d parameter(s) but file has no params:query block",
					name, len(doc.Parameters))
			}

			for paramName := range doc.Parameters {
				if !strings.Contains(s, paramName+":") {
					t.Errorf("%s: parameter %q is defined in API docs but missing from the file",
						name, paramName)
				}
			}
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk Bruno: %v", err)
	}
}

package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeAppScriptPushFiles(t *testing.T) {
	manifest := map[string]any{"name": "appsscript", "type": "JSON", "source": "{}"}
	raw := func(files ...map[string]any) string {
		t.Helper()
		b, err := json.Marshal(files)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	good := raw(manifest, map[string]any{"name": "Code", "type": "SERVER_JS", "source": ""})
	files, err := decodeAppScriptPushFiles(good)
	if err != nil || len(files) != 2 || files[1].Source != "" {
		t.Fatalf("valid empty source: %+v %v", files, err)
	}
	for _, input := range []string{
		`{}`, `null`, `[]`, good + `[]`,
		raw(map[string]any{"name": "Code", "type": "SERVER_JS", "source": "x"}),
		raw(manifest, map[string]any{"name": "Code", "type": "SERVER_JS"}),
		raw(manifest, manifest),
		raw(manifest, map[string]any{"name": "Code", "type": "SERVER_JS", "source": "x", "unknown": true}),
		raw(manifest, map[string]any{"name": "Code", "type": "OTHER", "source": "x"}),
		raw(map[string]any{"name": "appsscript", "type": "JSON", "source": "null"}),
		raw(map[string]any{"name": "appsscript", "type": "JSON", "source": "[]"}),
		raw(map[string]any{"name": "appsscript", "type": "JSON", "source": "{broken"}),
		raw(manifest, map[string]any{"name": "other", "type": "JSON", "source": "{}"}),
		raw(manifest, map[string]any{"name": "Code\n", "type": "SERVER_JS", "source": "x"}),
	} {
		if _, err := decodeAppScriptPushFiles(input); err == nil {
			t.Errorf("accepted invalid input %s", input)
		}
	}
}

func TestReadAppScriptPushFilesDirectory(t *testing.T) {
	dir := t.TempDir()
	for name, source := range map[string]string{"appsscript.json": "{}", "Code.gs": "function hello() {}", "View.html": "<b>Hello</b>", "package.json": "{}", ".secret.gs": "secret"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	files, err := readAppScriptPushFiles(dir, "")
	if err != nil || len(files) != 3 {
		t.Fatalf("files %+v error %v", files, err)
	}
	if files[0].Name != "Code" || files[0].Source != "function hello() {}" || files[1].Name != "View" || files[2].Name != "appsscript" {
		t.Fatalf("wrong mapping/order %+v", files)
	}
	if _, err := readAppScriptPushFiles(dir, `[]`); err == nil {
		t.Fatal("accepted both input sources")
	}
	if _, err := readAppScriptPushFiles("", ""); err == nil {
		t.Fatal("accepted missing inputs")
	}
	outside := filepath.Join(t.TempDir(), "secret.gs")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.gs")); err != nil {
		t.Fatal(err)
	}
	if _, err := readAppScriptPushFiles(dir, ""); err == nil {
		t.Fatal("read symlink outside project")
	}
}

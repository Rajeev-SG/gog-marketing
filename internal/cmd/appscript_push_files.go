package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	scriptapi "google.golang.org/api/script/v1"
)

func readAppScriptPushFiles(dir, inline string) (_ []*scriptapi.File, err error) {
	if (dir == "") == (inline == "") {
		return nil, fmt.Errorf("exactly one of dir or inline must be provided")
	}
	if inline != "" {
		return decodeAppScriptPushFiles(inline)
	}
	expanded, err := expandAppScriptDir(dir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(expanded)
	if err != nil {
		return nil, fmt.Errorf("open script directory %s: %w", expanded, err)
	}
	defer func() {
		if cerr := root.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close script directory %s: %w", expanded, cerr)
		}
	}()
	dirFile, err := root.Open(".")
	if err != nil {
		return nil, fmt.Errorf("read script directory %s: %w", expanded, err)
	}
	entries, err := dirFile.ReadDir(-1)
	closeErr := dirFile.Close()
	if err != nil {
		return nil, fmt.Errorf("read script directory %s: %w", expanded, err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close script directory %s: %w", expanded, closeErr)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	type pushFileDTO struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		Source string `json:"source"`
	}
	files := make([]pushFileDTO, 0, len(entries))
	total := 0
	for _, entry := range entries {
		base := entry.Name()
		if entry.IsDir() || strings.HasPrefix(base, ".") {
			continue
		}
		var kind string
		switch strings.ToLower(filepath.Ext(base)) {
		case ".gs", ".js":
			kind = "SERVER_JS"
		case ".html":
			kind = "HTML"
		case ".json":
			if base != "appsscript.json" {
				continue
			}
			kind = "JSON"
		default:
			continue
		}
		info, fileErr := entry.Info()
		if fileErr != nil {
			return nil, fmt.Errorf("stat script file %s: %w", base, fileErr)
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("script file %s: not a regular file", base)
		}
		file, fileErr := root.Open(base)
		if fileErr != nil {
			return nil, fmt.Errorf("open script file %s: %w", base, fileErr)
		}
		openedInfo, fileErr := file.Stat()
		if fileErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("stat script file %s: %w", base, fileErr)
		}
		if !openedInfo.Mode().IsRegular() || !os.SameFile(openedInfo, info) {
			_ = file.Close()
			return nil, fmt.Errorf("script file %s: not a regular file", base)
		}
		data, fileErr := io.ReadAll(io.LimitReader(file, int64((5<<20)-total+1)))
		fileCloseErr := file.Close()
		if fileErr != nil {
			return nil, fmt.Errorf("read script file %s: %w", base, fileErr)
		}
		if fileCloseErr != nil {
			return nil, fmt.Errorf("close script file %s: %w", base, fileCloseErr)
		}
		total += len(data)
		if total > 5<<20 {
			return nil, fmt.Errorf("script file %s exceeds 5MiB limit", base)
		}
		if len(files) >= 1000 {
			return nil, usage("more than 1000 project files")
		}
		files = append(files, pushFileDTO{
			Name:   strings.TrimSuffix(base, filepath.Ext(base)),
			Type:   kind,
			Source: string(data),
		})
	}
	blob, err := json.Marshal(files)
	if err != nil {
		return nil, fmt.Errorf("encode script files: %w", err)
	}
	return decodeAppScriptPushFiles(string(blob))
}

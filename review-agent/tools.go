package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func execReadFile(root, path string, rangeStart, rangeEnd int) ToolResult {
	safe, err := safePath(root, path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	info, err := os.Stat(safe)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}
	if info.IsDir() {
		return ToolResult{Content: "path is a directory, use list_dir instead", IsError: true}
	}
	if info.Size() > 1<<20 {
		return ToolResult{Content: "file exceeds 1MB limit", IsError: true}
	}

	data, err := os.ReadFile(safe)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	lines := strings.Split(string(data), "\n")
	if rangeStart > 0 || rangeEnd > 0 {
		if rangeStart < 1 {
			rangeStart = 1
		}
		if rangeEnd < 1 || rangeEnd > len(lines) {
			rangeEnd = len(lines)
		}
		if rangeStart > len(lines) {
			return ToolResult{Content: fmt.Sprintf("start line %d exceeds file length %d", rangeStart, len(lines)), IsError: true}
		}
		lines = lines[rangeStart-1 : rangeEnd]
	}

	return ToolResult{Content: strings.Join(lines, "\n")}
}

type GrepMatch struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func execGrep(root, pattern, path, glob string) ToolResult {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid regex: %s", err), IsError: true}
	}

	safe, err := safePath(root, path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	var matches []string
	count := 0
	const maxMatches = 200

	err = filepath.Walk(safe, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if _, pathErr := safePath(root, p); pathErr != nil {
			return nil
		}
		if glob != "" {
			matched, _ := filepath.Match(glob, info.Name())
			if !matched {
				return nil
			}
		}
		if info.Size() > 1<<20 {
			return nil
		}

		f, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer f.Close()

		rel, _ := filepath.Rel(root, p)
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if re.MatchString(scanner.Text()) {
				matches = append(matches, fmt.Sprintf("%s:%d: %s", rel, lineNum, scanner.Text()))
				count++
				if count >= maxMatches {
					matches = append(matches, fmt.Sprintf("... truncated at %d matches", maxMatches))
					return filepath.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{Content: "no matches found"}
	}
	return ToolResult{Content: strings.Join(matches, "\n")}
}

func execListDir(root, path string) ToolResult {
	safe, err := safePath(root, path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	entries, err := os.ReadDir(safe)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	var lines []string
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		lines = append(lines, e.Name()+suffix)
	}
	return ToolResult{Content: strings.Join(lines, "\n")}
}

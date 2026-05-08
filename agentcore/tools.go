package agentcore

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const MaxToolResultBytes = 32_000

type ToolResult struct {
	Content string `json:"content"`
	IsError bool   `json:"is_error"`
}

func (r ToolResult) Truncated() ToolResult {
	if len(r.Content) <= MaxToolResultBytes {
		return r
	}
	r.Content = r.Content[:MaxToolResultBytes] + "\n... truncated at 32KB ..."
	return r
}

func StandardToolDefs() []any {
	return []any{
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "read_file",
				"description": "Read a file's contents. Path is relative to workspace root.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":  map[string]any{"type": "string", "description": "File path relative to workspace root"},
						"start": map[string]any{"type": "integer", "description": "Start line (1-indexed, optional)"},
						"end":   map[string]any{"type": "integer", "description": "End line (inclusive, optional)"},
					},
					"required": []string{"path"},
				},
			},
		},
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "grep",
				"description": "Search for a regex pattern in files under a path.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Regex pattern to search for"},
						"path":    map[string]any{"type": "string", "description": "Directory or file to search, relative to workspace root"},
						"glob":    map[string]any{"type": "string", "description": "Filename glob filter (e.g. *.go), optional"},
					},
					"required": []string{"pattern", "path"},
				},
			},
		},
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "list_dir",
				"description": "List entries in a directory.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path": map[string]any{"type": "string", "description": "Directory path relative to workspace root"},
					},
					"required": []string{"path"},
				},
			},
		},
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "git_log",
				"description": "Show recent git commit history.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"count": map[string]any{"type": "integer", "description": "Number of commits to show (default 10, max 50)"},
						"ref":   map[string]any{"type": "string", "description": "Git ref to start from (e.g. HEAD, main). Optional."},
					},
				},
			},
		},
	}
}

func StandardToolDispatch(root string, tc LLMTool, contextPulled *[]string) ToolResult {
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid arguments: %s", err), IsError: true}
	}

	switch tc.Function.Name {
	case "read_file":
		path, _ := args["path"].(string)
		if path == "" {
			return ToolResult{Content: "missing required argument: path", IsError: true}
		}
		start, _ := args["start"].(float64)
		end, _ := args["end"].(float64)
		result := ExecReadFile(root, path, int(start), int(end))
		if !result.IsError {
			*contextPulled = append(*contextPulled, path)
		}
		return result.Truncated()

	case "grep":
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return ToolResult{Content: "missing required argument: pattern", IsError: true}
		}
		path, _ := args["path"].(string)
		if path == "" {
			path = "."
		}
		glob, _ := args["glob"].(string)
		result := ExecGrep(root, pattern, path, glob)
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("grep:%s in %s", pattern, path))
		}
		return result.Truncated()

	case "list_dir":
		path, _ := args["path"].(string)
		if path == "" {
			path = "."
		}
		result := ExecListDir(root, path)
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("ls:%s", path))
		}
		return result.Truncated()

	case "git_log":
		count, _ := args["count"].(float64)
		ref, _ := args["ref"].(string)
		result := ExecGitLog(root, int(count), ref)
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("git_log:%s", ref))
		}
		return result.Truncated()

	default:
		return ToolResult{Content: fmt.Sprintf("unknown tool: %s", tc.Function.Name), IsError: true}
	}
}

func ExecReadFile(root, path string, rangeStart, rangeEnd int) ToolResult {
	safe, err := SafePath(root, path)
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
		if rangeStart > rangeEnd {
			return ToolResult{Content: fmt.Sprintf("invalid range: start %d > end %d", rangeStart, rangeEnd), IsError: true}
		}
		lines = lines[rangeStart-1 : rangeEnd]
	}

	return ToolResult{Content: strings.Join(lines, "\n")}
}

func ExecGrep(root, pattern, path, glob string) ToolResult {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return ToolResult{Content: fmt.Sprintf("invalid regex: %s", err), IsError: true}
	}

	safe, err := SafePath(root, path)
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
		if _, pathErr := SafePath(root, p); pathErr != nil {
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

func ExecListDir(root, path string) ToolResult {
	safe, err := SafePath(root, path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	entries, err := os.ReadDir(safe)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	lines := make([]string, 0, len(entries))
	for _, e := range entries {
		suffix := ""
		if e.IsDir() {
			suffix = "/"
		}
		lines = append(lines, e.Name()+suffix)
	}
	return ToolResult{Content: strings.Join(lines, "\n")}
}

func ExecGitLog(root string, count int, ref string) ToolResult {
	if count <= 0 || count > 50 {
		count = 10
	}
	args := []string{"log", fmt.Sprintf("-%d", count), "--oneline", "--no-decorate"}
	if ref != "" {
		args = append(args, ref)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return ToolResult{Content: fmt.Sprintf("git log failed: %s", string(exitErr.Stderr)), IsError: true}
		}
		return ToolResult{Content: fmt.Sprintf("git log failed: %s", err), IsError: true}
	}
	if len(out) == 0 {
		return ToolResult{Content: "no commits found"}
	}
	return ToolResult{Content: string(out)}
}

func ToolCallSignature(calls []LLMTool) string {
	parts := make([]string, len(calls))
	for i, c := range calls {
		parts[i] = c.Function.Name + ":" + c.Function.Arguments
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

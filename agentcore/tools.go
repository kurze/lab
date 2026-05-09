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
				"name":        "glob",
				"description": "Find files by name pattern. Uses doublestar glob syntax: ** matches any number of directories, * matches within a single directory.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pattern": map[string]any{"type": "string", "description": "Glob pattern (e.g. **/*.go, src/**/test_*.py, **/config*.toml)"},
					},
					"required": []string{"pattern"},
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
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "git_diff",
				"description": "Show diff between two git refs, or for a specific file. Useful to see what changed between commits or branches.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"base": map[string]any{"type": "string", "description": "Base ref (commit, branch, tag). Default: HEAD~1"},
						"head": map[string]any{"type": "string", "description": "Head ref to compare against. Default: HEAD"},
						"path": map[string]any{"type": "string", "description": "Limit diff to a specific file path (optional)"},
						"stat": map[string]any{"type": "boolean", "description": "If true, show only file names and stats, not full diff"},
					},
				},
			},
		},
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "git_blame",
				"description": "Show who last modified each line of a file and when. Useful to understand authorship and recency of code.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"path":  map[string]any{"type": "string", "description": "File path relative to workspace root"},
						"start": map[string]any{"type": "integer", "description": "Start line (optional, 1-indexed)"},
						"end":   map[string]any{"type": "integer", "description": "End line (optional, inclusive)"},
					},
					"required": []string{"path"},
				},
			},
		},
		map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        "git_show",
				"description": "Show the contents of a specific git commit: message, author, date, and diff.",
				"parameters": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"ref":  map[string]any{"type": "string", "description": "Commit ref (hash, branch, tag, HEAD~N)"},
						"stat": map[string]any{"type": "boolean", "description": "If true, show only file stats, not full diff"},
					},
					"required": []string{"ref"},
				},
			},
		},
	}
}

func LoadAgentsMD(root, dir string, seen map[string]bool) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}

	var dirs []string
	for d := absDir; ; d = filepath.Dir(d) {
		dirs = append(dirs, d)
		if d == absRoot || !strings.HasPrefix(d, absRoot) {
			break
		}
		if d == filepath.Dir(d) {
			break
		}
	}

	var parts []string
	for i := len(dirs) - 1; i >= 0; i-- {
		d := dirs[i]
		if seen[d] {
			continue
		}
		seen[d] = true
		p := filepath.Join(d, "AGENTS.md")
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		if content == "" {
			continue
		}
		rel, _ := filepath.Rel(absRoot, d)
		if rel == "" || rel == "." {
			rel = "."
		}
		parts = append(parts, fmt.Sprintf("--- AGENTS.md (%s) ---\n%s\n--- end AGENTS.md ---", rel, content))
	}
	return strings.Join(parts, "\n\n")
}

func StandardToolDispatch(root string, tc LLMTool, contextPulled *[]string, seen map[string]bool) ToolResult {
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
			if seen != nil {
				dir := filepath.Dir(filepath.Join(root, path))
				if extra := LoadAgentsMD(root, dir, seen); extra != "" {
					result.Content += "\n\n" + extra
				}
			}
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
			if seen != nil {
				dir := filepath.Join(root, path)
				if extra := LoadAgentsMD(root, dir, seen); extra != "" {
					result.Content += "\n\n" + extra
				}
			}
		}
		return result.Truncated()

	case "glob":
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return ToolResult{Content: "missing required argument: pattern", IsError: true}
		}
		result := ExecGlob(root, pattern)
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("glob:%s", pattern))
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

	case "git_diff":
		base, _ := args["base"].(string)
		head, _ := args["head"].(string)
		path, _ := args["path"].(string)
		stat, _ := args["stat"].(bool)
		result := ExecGitDiff(root, base, head, path, stat)
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("git_diff:%s..%s", base, head))
		}
		return result.Truncated()

	case "git_blame":
		path, _ := args["path"].(string)
		if path == "" {
			return ToolResult{Content: "missing required argument: path", IsError: true}
		}
		start, _ := args["start"].(float64)
		end, _ := args["end"].(float64)
		result := ExecGitBlame(root, path, int(start), int(end))
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("git_blame:%s", path))
		}
		return result.Truncated()

	case "git_show":
		ref, _ := args["ref"].(string)
		if ref == "" {
			return ToolResult{Content: "missing required argument: ref", IsError: true}
		}
		stat, _ := args["stat"].(bool)
		result := ExecGitShow(root, ref, stat)
		if !result.IsError {
			*contextPulled = append(*contextPulled, fmt.Sprintf("git_show:%s", ref))
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

func ExecGlob(root, pattern string) ToolResult {
	const maxResults = 500

	var matches []string
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			base := info.Name()
			if base == ".git" || base == "node_modules" || base == "__pycache__" || base == ".venv" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if doublestarMatch(pattern, rel) {
			matches = append(matches, rel)
			if len(matches) >= maxResults {
				return filepath.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	if len(matches) == 0 {
		return ToolResult{Content: "no files found"}
	}
	result := strings.Join(matches, "\n")
	if len(matches) >= maxResults {
		result += fmt.Sprintf("\n... truncated at %d files", maxResults)
	}
	return ToolResult{Content: result}
}

func doublestarMatch(pattern, path string) bool {
	patParts := strings.Split(pattern, "/")
	pathParts := strings.Split(path, "/")
	return matchParts(patParts, pathParts)
}

func matchParts(pat, path []string) bool {
	for len(pat) > 0 && len(path) > 0 {
		if pat[0] == "**" {
			pat = pat[1:]
			if len(pat) == 0 {
				return true
			}
			for i := 0; i <= len(path); i++ {
				if matchParts(pat, path[i:]) {
					return true
				}
			}
			return false
		}
		matched, _ := filepath.Match(pat[0], path[0])
		if !matched {
			return false
		}
		pat = pat[1:]
		path = path[1:]
	}
	if len(pat) == 1 && pat[0] == "**" {
		return true
	}
	return len(pat) == 0 && len(path) == 0
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

func ExecGitDiff(root, base, head, path string, stat bool) ToolResult {
	if base == "" {
		base = "HEAD~1"
	}
	if head == "" {
		head = "HEAD"
	}
	args := []string{"diff", base, head}
	if stat {
		args = append(args, "--stat")
	}
	if path != "" {
		args = append(args, "--", path)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return ToolResult{Content: fmt.Sprintf("git diff failed: %s", string(exitErr.Stderr)), IsError: true}
		}
		return ToolResult{Content: fmt.Sprintf("git diff failed: %s", err), IsError: true}
	}
	if len(out) == 0 {
		return ToolResult{Content: "no differences found"}
	}
	return ToolResult{Content: string(out)}
}

func ExecGitBlame(root, path string, start, end int) ToolResult {
	safe, err := SafePath(root, path)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	rel, err := filepath.Rel(root, safe)
	if err != nil {
		return ToolResult{Content: err.Error(), IsError: true}
	}

	args := []string{"blame", "--porcelain"}
	if start > 0 && end >= start {
		args = append(args, fmt.Sprintf("-L%d,%d", start, end))
	} else if start > 0 {
		args = append(args, fmt.Sprintf("-L%d,+50", start))
	}
	args = append(args, rel)

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return ToolResult{Content: fmt.Sprintf("git blame failed: %s", string(exitErr.Stderr)), IsError: true}
		}
		return ToolResult{Content: fmt.Sprintf("git blame failed: %s", err), IsError: true}
	}
	if len(out) == 0 {
		return ToolResult{Content: "no blame output"}
	}

	return ToolResult{Content: summarizeBlame(string(out))}
}

func summarizeBlame(porcelain string) string {
	type blameInfo struct {
		author string
		date   string
		line   int
		text   string
	}

	var entries []blameInfo
	var current blameInfo
	lineNum := 0

	for _, raw := range strings.Split(porcelain, "\n") {
		switch {
		case strings.HasPrefix(raw, "author "):
			current.author = strings.TrimPrefix(raw, "author ")
		case strings.HasPrefix(raw, "author-time "):
			ts := strings.TrimPrefix(raw, "author-time ")
			if t, err := fmt.Sscanf(ts, "%d", new(int64)); err == nil && t > 0 {
				var unix int64
				fmt.Sscanf(ts, "%d", &unix)
				current.date = fmt.Sprintf("%d", unix)
			}
		case strings.HasPrefix(raw, "\t"):
			lineNum++
			current.line = lineNum
			current.text = strings.TrimPrefix(raw, "\t")
			entries = append(entries, current)
			current = blameInfo{}
		}
	}

	var b strings.Builder
	for _, e := range entries {
		fmt.Fprintf(&b, "%4d │ %-20s │ %s\n", e.line, e.author, e.text)
	}
	return b.String()
}

func ExecGitShow(root, ref string, stat bool) ToolResult {
	args := []string{"show", ref, "--no-decorate"}
	if stat {
		args = []string{"show", ref, "--no-decorate", "--stat"}
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return ToolResult{Content: fmt.Sprintf("git show failed: %s", string(exitErr.Stderr)), IsError: true}
		}
		return ToolResult{Content: fmt.Sprintf("git show failed: %s", err), IsError: true}
	}
	if len(out) == 0 {
		return ToolResult{Content: "no output"}
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

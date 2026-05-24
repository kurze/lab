package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kurze/lab/agentcore"
)

type traceEntryParsed struct {
	Timestamp        time.Time         `json:"ts"`
	Iteration        int               `json:"iteration"`
	Role             string            `json:"role"`
	Content          string            `json:"content,omitempty"`
	ToolCalls        json.RawMessage   `json:"tool_calls,omitempty"`
	ToolResults      json.RawMessage   `json:"tool_results,omitempty"`
	PromptTokens     int               `json:"prompt_tokens,omitempty"`
	CompletionTokens int               `json:"completion_tokens,omitempty"`
	TotalTokens      int               `json:"total_tokens,omitempty"`
	LatencyMs        int64             `json:"latency_ms,omitempty"`
	Meta             map[string]string `json:"meta,omitempty"`
}

func resolveTracesDir(cfg Config) string {
	if cfg.LogDir != "" {
		return cfg.LogDir
	}
	return agentcore.TracesDir(agentName)
}

func cmdLogs(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, `Usage: scrutineer logs <command> [flags]

Commands:
  list    List recent review trace sessions
  show    Display a trace session
  tail    Stream a running trace
  prune   Delete old trace files
`)
		os.Exit(1)
	}

	switch args[0] {
	case "list":
		cmdLogsList(args[1:])
	case "show":
		cmdLogsShow(args[1:])
	case "tail":
		cmdLogsTail(args[1:])
	case "prune":
		cmdLogsPrune(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown logs command: %s\n", args[0])
		os.Exit(1)
	}
}

type traceFile struct {
	Name string
	Path string
	Time time.Time
	Size int64
	Meta map[string]string
	Tag  string
}

func cmdLogsList(args []string) {
	fs := flag.NewFlagSet("logs list", flag.ExitOnError)
	limit := fs.Int("limit", 20, "max traces to show")
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	dir := resolveTracesDir(cfg)

	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "no traces found in %s\n", dir)
		return
	}

	var files []traceFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}

		tf := traceFile{
			Name: strings.TrimSuffix(e.Name(), ".jsonl"),
			Path: filepath.Join(dir, e.Name()),
			Size: info.Size(),
		}

		tf.Time, tf.Tag = parseTraceFilename(tf.Name)
		tf.Meta = readTraceMeta(tf.Path)
		files = append(files, tf)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Time.After(files[j].Time)
	})

	if *limit > 0 && len(files) > *limit {
		files = files[:*limit]
	}

	if len(files) == 0 {
		fmt.Println("no traces found")
		return
	}

	fmt.Printf("%-19s  %-20s  %-20s  %-8s  %s\n",
		"DATE", "TAG", "TARGET", "SIZE", "ID")
	fmt.Printf("%-19s  %-20s  %-20s  %-8s  %s\n",
		"───────────────────", "────────────────────", "────────────────────", "────────", "──────────────────────────────")

	for _, f := range files {
		target := formatTarget(f.Meta)
		size := formatSize(f.Size)
		date := f.Time.Format("2006-01-02 15:04:05")
		tag := f.Tag
		if len(tag) > 20 {
			tag = tag[:20]
		}
		if len(target) > 20 {
			target = target[:20]
		}
		fmt.Printf("%-19s  %-20s  %-20s  %-8s  %s\n",
			cl(ansiDim, date), tag, target, cl(ansiDim, size), cl(ansiDim, f.Name))
	}
}

func cmdLogsShow(args []string) {
	fs := flag.NewFlagSet("logs show", flag.ExitOnError)
	full := fs.Bool("full", false, "show full content (don't truncate)")
	jsonOut := fs.Bool("json", false, "output raw JSONL")
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	if fs.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "usage: scrutineer logs show <id>\n")
		os.Exit(1)
	}

	cfg := loadConfig(*configPath)
	id := fs.Arg(0)
	path := findTraceFile(resolveTracesDir(cfg), id)
	if path == "" {
		fmt.Fprintf(os.Stderr, "trace not found: %s\n", id)
		os.Exit(1)
	}

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if *jsonOut {
			fmt.Println(string(line))
			continue
		}

		var entry traceEntryParsed
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		printTraceEntry(entry, *full)
	}
}

func cmdLogsTail(args []string) {
	fs := flag.NewFlagSet("logs tail", flag.ExitOnError)
	id := fs.String("id", "", "trace ID to tail (default: most recent)")
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	dir := resolveTracesDir(cfg)

	var path string
	if *id != "" {
		path = findTraceFile(dir, *id)
		if path == "" {
			fmt.Fprintf(os.Stderr, "trace not found: %s\n", *id)
			os.Exit(1)
		}
	} else {
		path = mostRecentTrace(dir)
		if path == "" {
			fmt.Fprintf(os.Stderr, "no traces found in %s\n", dir)
			os.Exit(1)
		}
	}

	logf("tailing %s", filepath.Base(path))

	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var entry traceEntryParsed
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		printTraceEntry(entry, false)
	}
}

func cmdLogsPrune(args []string) {
	fs := flag.NewFlagSet("logs prune", flag.ExitOnError)
	maxAge := fs.Int("max-age", 0, "max age in days (0 = use config default)")
	maxSize := fs.Int("max-size", 0, "max total size in MB (0 = use config default)")
	configPath := fs.String("config", "", "path to config file")
	fs.Parse(args)

	cfg := loadConfig(*configPath)
	dir := resolveTracesDir(cfg)

	ageDays := cfg.LogMaxAgeDays
	if *maxAge > 0 {
		ageDays = *maxAge
	}
	sizeMB := cfg.LogMaxSizeMB
	if *maxSize > 0 {
		sizeMB = *maxSize
	}

	deleted, freed := pruneTraces(dir, ageDays, sizeMB)
	if deleted == 0 {
		logf("no traces to prune")
	} else {
		logf("pruned %d traces, freed %s", deleted, formatSize(freed))
	}
}

func pruneTraces(dir string, maxAgeDays int, maxSizeMB int) (deleted int, freed int64) {
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	type fileInfo struct {
		path    string
		modTime time.Time
		size    int64
	}

	var files []fileInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    filepath.Join(dir, e.Name()),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	cutoff := time.Now().AddDate(0, 0, -maxAgeDays)
	for i := range files {
		if files[i].modTime.Before(cutoff) {
			if os.Remove(files[i].path) == nil {
				deleted++
				freed += files[i].size
				files[i].size = 0
			}
		}
	}

	if maxSizeMB > 0 {
		maxBytes := int64(maxSizeMB) * 1024 * 1024
		var total int64
		for _, f := range files {
			total += f.size
		}
		for i := range files {
			if total <= maxBytes {
				break
			}
			if files[i].size == 0 {
				continue
			}
			if os.Remove(files[i].path) == nil {
				deleted++
				freed += files[i].size
				total -= files[i].size
			}
		}
	}

	return
}

func parseTraceFilename(name string) (time.Time, string) {
	// Format: 20060102-150405-tag
	if len(name) < 15 {
		return time.Time{}, name
	}
	t, err := time.ParseInLocation("20060102-150405", name[:15], time.Local)
	if err != nil {
		return time.Time{}, name
	}
	tag := ""
	if len(name) > 16 {
		tag = name[16:]
	}
	return t, tag
}

func readTraceMeta(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if scanner.Scan() {
		var entry traceEntryParsed
		if err := json.Unmarshal(scanner.Bytes(), &entry); err == nil && len(entry.Meta) > 0 {
			return entry.Meta
		}
	}
	return nil
}

func findTraceFile(dir, id string) string {
	// Try exact match first
	path := filepath.Join(dir, id+".jsonl")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	// Try prefix match
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), id) && strings.HasSuffix(e.Name(), ".jsonl") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

func mostRecentTrace(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var latest string
	var latestTime time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(latestTime) {
			latestTime = info.ModTime()
			latest = filepath.Join(dir, e.Name())
		}
	}
	return latest
}

func formatTarget(meta map[string]string) string {
	if id := meta["mr_id"]; id != "" {
		return "MR#" + id
	}
	if b := meta["branch"]; b != "" {
		return "branch:" + b
	}
	if c := meta["commit"]; c != "" {
		if len(c) > 8 {
			c = c[:8]
		}
		return "commit:" + c
	}
	return ""
}

func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fM", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.0fK", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

func printTraceEntry(e traceEntryParsed, full bool) {
	color := roleColor(e.Role)
	ts := cl(ansiDim, e.Timestamp.Format("15:04:05"))

	content := e.Content
	if !full && len(content) > 200 {
		content = content[:200] + "…"
	}
	content = strings.ReplaceAll(content, "\n", "\n    ")

	var tokens string
	if e.TotalTokens > 0 {
		tokens = fmt.Sprintf(" %s", cl(ansiDim, formatTokens(e.TotalTokens)))
	}
	var latency string
	if e.LatencyMs > 0 {
		latency = fmt.Sprintf(" %s", cl(ansiDim, fmt.Sprintf("%dms", e.LatencyMs)))
	}

	if len(e.ToolCalls) > 0 && string(e.ToolCalls) != "null" {
		var calls []struct {
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if json.Unmarshal(e.ToolCalls, &calls) == nil && len(calls) > 0 {
			names := make([]string, len(calls))
			for i, c := range calls {
				names[i] = c.Function.Name
			}
			content = fmt.Sprintf("→ %s", strings.Join(names, ", "))
			if e.Content != "" && full {
				content += "\n    " + strings.ReplaceAll(e.Content, "\n", "\n    ")
			}
		}
	}

	fmt.Printf("%s [%d] %s: %s%s%s\n",
		ts, e.Iteration, cl(color, e.Role), content, tokens, latency)

	if len(e.Meta) > 0 {
		var parts []string
		for k, v := range e.Meta {
			parts = append(parts, fmt.Sprintf("%s=%s", k, v))
		}
		sort.Strings(parts)
		fmt.Printf("    %s\n", cl(ansiDim, strings.Join(parts, " ")))
	}
}

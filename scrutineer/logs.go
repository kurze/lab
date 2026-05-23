package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kurze/lab/agentcore"
)

func cmdLogs(args []string) {
	subcmd := "list"
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		subcmd = args[0]
		args = args[1:]
	}

	switch subcmd {
	case "list":
		fs := flag.NewFlagSet("logs list", flag.ExitOnError)
		verbose := fs.Bool("v", false, "show token counts and metadata")
		n := fs.Int("n", 20, "number of entries to show")
		tag := fs.String("tag", "", "filter by tag (e.g. mr-review, commit-review)")
		fs.Parse(args)

		files, err := listTraceFiles()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if *tag != "" {
			var filtered []traceFile
			for _, f := range files {
				if strings.Contains(f.Tag, *tag) {
					filtered = append(filtered, f)
				}
			}
			files = filtered
		}
		if *n > 0 && len(files) > *n {
			files = files[:*n]
		}
		printTraceList(files, *verbose)

	case "show":
		fs := flag.NewFlagSet("logs show", flag.ExitOnError)
		fs.Parse(args)
		target := fs.Arg(0)
		if target == "" {
			fmt.Fprintf(os.Stderr, "usage: scrutineer logs show <filename|index>\n")
			os.Exit(1)
		}
		tf, err := resolveTraceFile(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := printTraceShow(*tf); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "unknown logs subcommand: %s\nusage: scrutineer logs [list|show] [flags]\n", subcmd)
		os.Exit(1)
	}
}

func resolveTraceFile(target string) (*traceFile, error) {
	var idx int
	if _, err := fmt.Sscanf(target, "%d", &idx); err == nil && idx > 0 {
		files, err := listTraceFiles()
		if err != nil {
			return nil, err
		}
		if idx > len(files) {
			return nil, fmt.Errorf("index %d out of range (have %d files)", idx, len(files))
		}
		return &files[idx-1], nil
	}

	files, err := listTraceFiles()
	if err != nil {
		return nil, err
	}
	for i := range files {
		if files[i].Name == target || strings.Contains(files[i].Name, target) {
			return &files[i], nil
		}
	}

	return nil, fmt.Errorf("no trace file matching %q", target)
}

type traceFile struct {
	Path      string
	Name      string
	Size      int64
	ModTime   time.Time
	Tag       string
	Timestamp time.Time
}

func listTraceFiles() ([]traceFile, error) {
	dir := agentcore.TracesDir(agentName)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
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

		ts, tag := parseTraceFilename(e.Name())
		files = append(files, traceFile{
			Path:      filepath.Join(dir, e.Name()),
			Name:      e.Name(),
			Size:      info.Size(),
			ModTime:   info.ModTime(),
			Tag:       tag,
			Timestamp: ts,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Timestamp.After(files[j].Timestamp)
	})
	return files, nil
}

func parseTraceFilename(name string) (time.Time, string) {
	name = strings.TrimSuffix(name, ".jsonl")
	// format: 20060102-150405-tag
	if len(name) < 16 {
		return time.Time{}, name
	}
	ts, err := time.ParseInLocation("20060102-150405", name[:15], time.Local)
	if err != nil {
		return time.Time{}, name
	}
	tag := strings.TrimPrefix(name[15:], "-")
	return ts, tag
}

type traceSummary struct {
	File             traceFile
	Meta             map[string]string
	Iterations       int
	TotalTokens      int
	PromptTokens     int
	CompletionTokens int
	TotalLatencyMs   int64
	Duration         time.Duration
	rawData          []byte
}

func summarizeTrace(tf traceFile) (*traceSummary, error) {
	return summarizeTraceData(tf, nil)
}

func summarizeTraceData(tf traceFile, data []byte) (*traceSummary, error) {
	if data == nil {
		var err error
		data, err = os.ReadFile(tf.Path)
		if err != nil {
			return nil, err
		}
	}

	s := &traceSummary{File: tf, rawData: data}
	var firstTS, lastTS time.Time

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}

		var entry struct {
			Timestamp        time.Time         `json:"ts"`
			Iteration        int               `json:"iteration"`
			Role             string            `json:"role"`
			Meta             map[string]string `json:"meta,omitempty"`
			PromptTokens     int               `json:"prompt_tokens"`
			CompletionTokens int               `json:"completion_tokens"`
			TotalTokens      int               `json:"total_tokens"`
			LatencyMs        int64             `json:"latency_ms"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		if firstTS.IsZero() {
			firstTS = entry.Timestamp
		}
		lastTS = entry.Timestamp

		if entry.Meta != nil && s.Meta == nil {
			s.Meta = entry.Meta
		}

		if entry.Iteration > s.Iterations {
			s.Iterations = entry.Iteration
		}

		s.PromptTokens += entry.PromptTokens
		s.CompletionTokens += entry.CompletionTokens
		if entry.TotalTokens > s.TotalTokens {
			s.TotalTokens = entry.TotalTokens
		}

		s.TotalLatencyMs += entry.LatencyMs
	}

	if !firstTS.IsZero() && !lastTS.IsZero() {
		s.Duration = lastTS.Sub(firstTS)
	}

	return s, nil
}

func printTraceList(files []traceFile, verbose bool) {
	if len(files) == 0 {
		fmt.Println("no trace files found")
		return
	}

	for _, f := range files {
		if !verbose {
			fmt.Printf("%-20s %-20s %s\n",
				f.Timestamp.Format("2006-01-02 15:04:05"),
				f.Tag,
				formatSize(f.Size))
			continue
		}

		s, err := summarizeTrace(f)
		if err != nil {
			fmt.Printf("%-20s %-20s %s  (error reading)\n",
				f.Timestamp.Format("2006-01-02 15:04:05"),
				f.Tag,
				formatSize(f.Size))
			continue
		}

		meta := ""
		if s.Meta != nil {
			if id, ok := s.Meta["mr_id"]; ok {
				meta = fmt.Sprintf(" mr:#%s", id)
			} else if b, ok := s.Meta["branch"]; ok {
				meta = fmt.Sprintf(" branch:%s", b)
			} else if c, ok := s.Meta["commit"]; ok {
				short := c
				if len(short) > 8 {
					short = short[:8]
				}
				meta = fmt.Sprintf(" commit:%s", short)
			}
		}

		fmt.Printf("%-20s %-20s %s  %d iter  %d tok  %s%s\n",
			f.Timestamp.Format("2006-01-02 15:04:05"),
			f.Tag,
			formatSize(f.Size),
			s.Iterations,
			s.TotalTokens,
			formatDuration(s.Duration),
			meta)
	}
}

func printTraceShow(tf traceFile) error {
	data, err := os.ReadFile(tf.Path)
	if err != nil {
		return err
	}

	s, err := summarizeTraceData(tf, data)
	if err != nil {
		return err
	}

	fmt.Printf("File:       %s\n", tf.Name)
	fmt.Printf("Tag:        %s\n", tf.Tag)
	fmt.Printf("Time:       %s\n", tf.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Printf("Size:       %s\n", formatSize(tf.Size))
	fmt.Printf("Iterations: %d\n", s.Iterations)
	fmt.Printf("Tokens:     %d prompt + %d completion = %d total\n",
		s.PromptTokens, s.CompletionTokens, s.TotalTokens)
	fmt.Printf("LLM time:   %dms\n", s.TotalLatencyMs)
	fmt.Printf("Wall time:  %s\n", formatDuration(s.Duration))

	if len(s.Meta) > 0 {
		fmt.Printf("Metadata:\n")
		for k, v := range s.Meta {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}

	fmt.Println()

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var entry struct {
			Timestamp time.Time `json:"ts"`
			Iteration int       `json:"iteration"`
			Role      string    `json:"role"`
			Content   string    `json:"content"`
			ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
			LatencyMs int64     `json:"latency_ms"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		latency := ""
		if entry.LatencyMs > 0 {
			latency = fmt.Sprintf(" (%dms)", entry.LatencyMs)
		}

		content := entry.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}

		toolInfo := ""
		if len(entry.ToolCalls) > 0 && string(entry.ToolCalls) != "null" {
			var calls []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			}
			if json.Unmarshal(entry.ToolCalls, &calls) == nil && len(calls) > 0 {
				names := make([]string, len(calls))
				for i, c := range calls {
					names[i] = c.Function.Name
				}
				toolInfo = fmt.Sprintf(" → %s", strings.Join(names, ", "))
			}
		}

		fmt.Printf("  [%d] %-10s%s%s %s\n",
			entry.Iteration, entry.Role, latency, toolInfo, content)
	}

	return nil
}

func formatSize(b int64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(b)/float64(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(b)/float64(1<<10))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%.1fm", d.Minutes())
}

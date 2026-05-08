package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	cfg := loadConfig()
	llm := newLLMClient(cfg)

	s := server.NewMCPServer(
		"wlx-review-agent",
		"0.1.0",
	)

	modelNames := make([]string, 0, len(cfg.Models))
	for k := range cfg.Models {
		modelNames = append(modelNames, k)
	}

	tool := mcp.NewTool("independent_review",
		mcp.WithDescription("Independently review an architectural artifact using a local LLM. The local model reads the artifact and explores the workspace on its own, then produces structured findings. Output is descriptive, never prescriptive."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("artifact_path",
			mcp.Description("Path to the artifact to review"),
			mcp.Required(),
		),
		mcp.WithString("focus",
			mcp.Description("Focus area for the review (e.g. 'security assumptions', 'missing error cases', 'consistency with RFC 8693')"),
			mcp.Required(),
		),
		mcp.WithString("workspace_root",
			mcp.Description("Root directory for workspace exploration. Defaults to the artifact's parent directory."),
		),
		mcp.WithString("model",
			mcp.Description(fmt.Sprintf("Model to use for review. Available: %s. Default: %s.", strings.Join(modelNames, ", "), cfg.DefaultModel)),
		),
		mcp.WithNumber("max_iterations",
			mcp.Description("Maximum agent iterations. Default 12."),
		),
	)

	s.AddTool(tool, handleReview(cfg, llm))

	stdio := server.NewStdioServer(s)
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func handleReview(cfg Config, llm *LLMClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		artifactPath, ok := args["artifact_path"].(string)
		if !ok || artifactPath == "" {
			return mcp.NewToolResultError("artifact_path is required"), nil
		}

		focus, ok := args["focus"].(string)
		if !ok || focus == "" {
			return mcp.NewToolResultError("focus is required"), nil
		}

		workspaceRoot, _ := args["workspace_root"].(string)
		if workspaceRoot == "" {
			workspaceRoot = filepath.Dir(artifactPath)
		}

		modelName, _ := args["model"].(string)
		model, err := cfg.ResolveModel(modelName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		maxIter := defaultMaxIter
		if v, ok := args["max_iterations"].(float64); ok && v > 0 {
			maxIter = int(v)
		}

		root, err := canonicalRoot(workspaceRoot)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid workspace root: %s", err)), nil
		}

		absArtifact, err := filepath.Abs(artifactPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid artifact path: %s", err)), nil
		}

		relArtifact, err := filepath.Rel(root, absArtifact)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("artifact not under workspace root: %s", err)), nil
		}

		result, err := runAgent(ctx, llm, model, root, relArtifact, focus, maxIter)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("review failed: %s", err)), nil
		}

		data, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal result: %s", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func init() {
	log.SetOutput(os.Stderr)
}

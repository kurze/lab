package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/kurze/lab/agentcore"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	cfg := loadConfig()
	llm := agentcore.NewLLMClient(cfg.LLMURL)

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

	grillTool := mcp.NewTool("grill",
		mcp.WithDescription("Generate tough, probing questions about an artifact by having a local LLM independently read it and the surrounding codebase. Returns questions designed to expose hidden assumptions, gaps, and risks. Like having a senior reviewer grill you on your design."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("artifact_path",
			mcp.Description("Path to the artifact to grill on"),
			mcp.Required(),
		),
		mcp.WithString("focus",
			mcp.Description("Focus area (e.g. 'security model', 'error handling strategy', 'deployment assumptions')"),
			mcp.Required(),
		),
		mcp.WithString("workspace_root",
			mcp.Description("Root directory for workspace exploration. Defaults to the artifact's parent directory."),
		),
		mcp.WithString("model",
			mcp.Description(fmt.Sprintf("Model to use. Available: %s. Default: %s.", strings.Join(modelNames, ", "), cfg.DefaultModel)),
		),
		mcp.WithNumber("max_iterations",
			mcp.Description("Maximum agent iterations. Default 12."),
		),
	)

	s.AddTool(grillTool, handleGrill(cfg, llm))

	diffTool := mcp.NewTool("diff_review",
		mcp.WithDescription("Review a git diff using a local LLM. The model reads the diff and explores the workspace for context, then produces structured findings focused on the changes. Supports any git diff reference: HEAD (uncommitted), commit..commit ranges, or branch names."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("workspace_root",
			mcp.Description("Root directory of the git repository."),
			mcp.Required(),
		),
		mcp.WithString("diff_ref",
			mcp.Description("Git diff reference. Examples: 'HEAD' (uncommitted changes), 'HEAD~3..HEAD' (last 3 commits), 'main..feature-branch'. Default: HEAD."),
		),
		mcp.WithString("focus",
			mcp.Description("Focus area for the review (e.g. 'error handling', 'security', 'correctness'). Default: general code review."),
		),
		mcp.WithString("model",
			mcp.Description(fmt.Sprintf("Model to use. Available: %s. Default: %s.", strings.Join(modelNames, ", "), cfg.DefaultModel)),
		),
		mcp.WithNumber("max_iterations",
			mcp.Description("Maximum agent iterations. Default 12."),
		),
	)

	s.AddTool(diffTool, handleDiffReviewMCP(cfg, llm))

	compareTool := mcp.NewTool("compare_artifacts",
		mcp.WithDescription("Compare two versions of a document (spec, ADR, design doc) using a local LLM. Produces structured findings about what changed and what the changes mean: added/removed/weakened assumptions, contradictions, new risks."),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(false),
		mcp.WithString("old_path",
			mcp.Description("Path to the old version of the artifact"),
			mcp.Required(),
		),
		mcp.WithString("new_path",
			mcp.Description("Path to the new version of the artifact"),
			mcp.Required(),
		),
		mcp.WithString("focus",
			mcp.Description("Focus area (e.g. 'security assumptions', 'API contract changes', 'deployment requirements')"),
			mcp.Required(),
		),
		mcp.WithString("workspace_root",
			mcp.Description("Root directory for workspace exploration. Defaults to the new artifact's parent directory."),
		),
		mcp.WithString("model",
			mcp.Description(fmt.Sprintf("Model to use. Available: %s. Default: %s.", strings.Join(modelNames, ", "), cfg.DefaultModel)),
		),
		mcp.WithNumber("max_iterations",
			mcp.Description("Maximum agent iterations. Default 12."),
		),
	)

	s.AddTool(compareTool, handleCompare(cfg, llm))

	stdio := server.NewStdioServer(s)
	if err := stdio.Listen(context.Background(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func handleReview(cfg Config, llm *agentcore.LLMClient) server.ToolHandlerFunc {
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

		root, err := agentcore.CanonicalRoot(workspaceRoot)
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

func handleGrill(cfg Config, llm *agentcore.LLMClient) server.ToolHandlerFunc {
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

		root, err := agentcore.CanonicalRoot(workspaceRoot)
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

		result, err := runGrill(ctx, llm, model, root, relArtifact, focus, maxIter)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("grill failed: %s", err)), nil
		}

		data, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal result: %s", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleCompare(cfg Config, llm *agentcore.LLMClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		oldPath, ok := args["old_path"].(string)
		if !ok || oldPath == "" {
			return mcp.NewToolResultError("old_path is required"), nil
		}

		newPath, ok := args["new_path"].(string)
		if !ok || newPath == "" {
			return mcp.NewToolResultError("new_path is required"), nil
		}

		focus, ok := args["focus"].(string)
		if !ok || focus == "" {
			return mcp.NewToolResultError("focus is required"), nil
		}

		workspaceRoot, _ := args["workspace_root"].(string)
		if workspaceRoot == "" {
			workspaceRoot = filepath.Dir(newPath)
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

		root, err := agentcore.CanonicalRoot(workspaceRoot)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid workspace root: %s", err)), nil
		}

		absOld, err := filepath.Abs(oldPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid old path: %s", err)), nil
		}
		relOld, err := filepath.Rel(root, absOld)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("old artifact not under workspace root: %s", err)), nil
		}

		absNew, err := filepath.Abs(newPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid new path: %s", err)), nil
		}
		relNew, err := filepath.Rel(root, absNew)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("new artifact not under workspace root: %s", err)), nil
		}

		result, err := runCompare(ctx, llm, model, root, relOld, relNew, focus, maxIter)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("compare failed: %s", err)), nil
		}

		data, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("marshal result: %s", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func handleDiffReviewMCP(cfg Config, llm *agentcore.LLMClient) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := req.GetArguments()

		workspaceRoot, ok := args["workspace_root"].(string)
		if !ok || workspaceRoot == "" {
			return mcp.NewToolResultError("workspace_root is required"), nil
		}

		diffRef, _ := args["diff_ref"].(string)
		ref := parseDiffRef(diffRef)
		if ref == "" && diffRef != "" {
			return mcp.NewToolResultError("invalid diff_ref: contains disallowed characters"), nil
		}
		if ref == "" {
			ref = "HEAD"
		}

		focus, _ := args["focus"].(string)
		if focus == "" {
			focus = "general code review: correctness, error handling, edge cases, consistency"
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

		root, err := agentcore.CanonicalRoot(workspaceRoot)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("invalid workspace root: %s", err)), nil
		}

		result, err := runDiffReview(ctx, llm, model, root, ref, focus, maxIter)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("diff review failed: %s", err)), nil
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

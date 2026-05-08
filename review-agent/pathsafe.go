package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func canonicalRoot(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("eval symlinks on workspace root: %w", err)
	}
	return resolved, nil
}

func safePath(root, requested string) (string, error) {
	joined := requested
	if !filepath.IsAbs(requested) {
		joined = filepath.Join(root, requested)
	}

	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", fmt.Errorf("resolve path: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet — check the parent directory instead.
			parent := filepath.Dir(abs)
			realParent, perr := filepath.EvalSymlinks(parent)
			if perr != nil {
				return "", fmt.Errorf("resolve parent path: %w", perr)
			}
			if !strings.HasPrefix(realParent+string(filepath.Separator), root+string(filepath.Separator)) && realParent != root {
				return "", fmt.Errorf("path escapes workspace root: %s", requested)
			}
			return abs, nil
		}
		return "", fmt.Errorf("resolve path: %w", err)
	}

	if !strings.HasPrefix(resolved+string(filepath.Separator), root+string(filepath.Separator)) && resolved != root {
		return "", fmt.Errorf("path escapes workspace root: %s", requested)
	}

	return resolved, nil
}

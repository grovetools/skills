package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// FindBinary is a helper to find the binary path for tests.
// It checks in the following order:
// 1. GROVE_SKILLS_BINARY environment variable
// 2. Common relative paths from test execution directory
// 3. System PATH
func FindBinary() (string, error) {
	// Check environment variable first
	if binary := os.Getenv("GROVE_SKILLS_BINARY"); binary != "" {
		return binary, nil
	}

	// Try common locations relative to test execution directory
	// Note: The binary may be named "skills" or "grove-skills" depending on build
	candidates := []string{
		"./bin/skills",
		"./bin/grove-skills",
		"../bin/skills",
		"../bin/grove-skills",
		"../../bin/skills",
		"../../bin/grove-skills",
		"../../../bin/skills",
		"../../../bin/grove-skills",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			absPath, err := filepath.Abs(candidate)
			if err != nil {
				return "", err
			}
			return absPath, nil
		}
	}

	// Try to find in PATH
	if path, err := exec.LookPath("grove-skills"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("could not find grove-skills binary - please set GROVE_SKILLS_BINARY environment variable or ensure grove-skills is built and in PATH")
}

// FindDaemonBinary locates the groved binary for tests.
// It checks GROVED_BINARY env var, common relative paths, and system PATH.
func FindDaemonBinary() (string, error) {
	if binary := os.Getenv("GROVED_BINARY"); binary != "" {
		return binary, nil
	}

	candidates := []string{
		"./bin/groved",
		"../bin/groved",
		"../../bin/groved",
		"../../../bin/groved",
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			absPath, err := filepath.Abs(candidate)
			if err != nil {
				return "", err
			}
			return absPath, nil
		}
	}

	if path, err := exec.LookPath("groved"); err == nil {
		return path, nil
	}

	return "", fmt.Errorf("could not find groved binary - please set GROVED_BINARY environment variable")
}

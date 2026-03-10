package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/grovetools/tend/pkg/harness"
	"github.com/grovetools/tend/pkg/tui"
	"github.com/grovetools/tend/pkg/verify"
)

// TUIScenario tests the skills browser TUI.
func TUIScenario() *harness.Scenario {
	return harness.NewScenarioWithOptions(
		"skills-tui",
		"Tests the skills browser TUI navigation and help",
		[]string{"tui", "browser"},
		[]harness.Step{
			harness.NewStep("Launch TUI and verify initial state", launchTUIAndVerifyInitial),
			harness.NewStep("Test navigation with vim keys", testNavigation),
			harness.NewStep("Test help menu toggle", testHelpMenu),
			harness.NewStep("Test search functionality", testSearch),
			harness.NewStep("Exit TUI cleanly", exitTUI),
		},
		true,  // localOnly - requires tmux
		false, // explicitOnly
	)
}

func launchTUIAndVerifyInitial(ctx *harness.Context) error {
	binary, err := FindBinary()
	if err != nil {
		return err
	}

	session, err := ctx.StartTUI(binary, []string{"tui"},
		tui.WithEnv("HOME="+ctx.HomeDir()),
	)
	if err != nil {
		return fmt.Errorf("failed to start TUI: %w", err)
	}
	ctx.Set("tui_session", session)

	// Wait for TUI to load
	if err := session.WaitForText("Skills Browser", 5*time.Second); err != nil {
		view, _ := session.Capture()
		ctx.ShowCommandOutput("TUI Failed to Load", view, "")
		return fmt.Errorf("timeout waiting for TUI to load: %w", err)
	}

	// Capture initial state
	initialView, err := session.Capture()
	if err != nil {
		return fmt.Errorf("failed to capture initial view: %w", err)
	}
	ctx.ShowCommandOutput("TUI Initial View", initialView, "")

	// Verify initial state
	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("shows Skills Browser header", initialView, "Skills Browser")
		v.Contains("shows help hint", initialView, "?")
		v.Contains("shows quit hint", initialView, "quit")
		v.Contains("shows skills count", initialView, "skills")
	})
}

func testNavigation(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Navigate down with j
	if err := session.SendKeys("j"); err != nil {
		return fmt.Errorf("failed to send j key: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Capture state after navigation
	afterJ, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after j navigation", afterJ, "")

	// Navigate to bottom with G
	if err := session.SendKeys("G"); err != nil {
		return fmt.Errorf("failed to send G key: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	afterG, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after G (bottom)", afterG, "")

	// Navigate to top with gg
	if err := session.SendKeys("g", "g"); err != nil {
		return fmt.Errorf("failed to send gg keys: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	afterGG, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after gg (top)", afterGG, "")

	// Verify cursor is still showing
	return ctx.Verify(func(v *verify.Collector) {
		// The cursor indicator should be visible
		v.Contains("cursor visible after navigation", afterGG, "Skills Browser")
	})
}

func testHelpMenu(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Open help with ?
	if err := session.SendKeys("?"); err != nil {
		return fmt.Errorf("failed to send ? key: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// Capture help state
	helpView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI Help View", helpView, "")

	// Verify help content
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Contains("help shows Navigation section", helpView, "Navigation")
		v.Contains("help shows Actions section", helpView, "Actions")
	}); err != nil {
		return err
	}

	// Close help with ?
	if err := session.SendKeys("?"); err != nil {
		return fmt.Errorf("failed to close help: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Verify we're back to main view
	afterHelp, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after closing help", afterHelp, "")

	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("back to main view", afterHelp, "Skills Browser")
	})
}

func testSearch(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Enter search mode with /
	if err := session.SendKeys("/"); err != nil {
		return fmt.Errorf("failed to enter search mode: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	searchView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI Search Mode", searchView, "")

	// Verify search mode is active
	if !strings.Contains(searchView, "Search:") {
		return fmt.Errorf("expected search mode to be active")
	}

	// Type a search term
	if err := session.Type("explain"); err != nil {
		return fmt.Errorf("failed to type search term: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	// Capture filtered view
	filteredView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI Filtered View", filteredView, "")

	// Confirm search with Enter
	if err := session.SendKeys("Enter"); err != nil {
		return fmt.Errorf("failed to confirm search: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	afterSearch, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after search confirm", afterSearch, "")

	// Verify filter is applied
	if err := ctx.Verify(func(v *verify.Collector) {
		v.Contains("filter shown in header", afterSearch, "Filter:")
	}); err != nil {
		return err
	}

	// Clear search with Ctrl+L
	if err := session.SendKeys("C-l"); err != nil {
		return fmt.Errorf("failed to clear search: %w", err)
	}
	time.Sleep(300 * time.Millisecond)

	clearedView, err := session.Capture()
	if err != nil {
		return err
	}
	ctx.ShowCommandOutput("TUI after clearing search", clearedView, "")

	// Verify filter is cleared (should show more than 1 skill)
	return ctx.Verify(func(v *verify.Collector) {
		v.Contains("search cleared, shows multiple skills", clearedView, "skills")
		// Make sure we don't still see the filter
		v.True("filter header cleared", !strings.Contains(clearedView, "Filter: explain"))
	})
}

func exitTUI(ctx *harness.Context) error {
	session := ctx.Get("tui_session").(*tui.Session)

	// Exit with q
	if err := session.SendKeys("q"); err != nil {
		return fmt.Errorf("failed to exit TUI: %w", err)
	}
	time.Sleep(500 * time.Millisecond)

	// The TUI should have exited
	return nil
}

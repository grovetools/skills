package browser

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/grovetools/core/tui/components/help"
	"github.com/grovetools/core/tui/embed"
	"github.com/grovetools/core/tui/keymap"
	"github.com/grovetools/core/tui/theme"
	skillskeymap "github.com/grovetools/skills/pkg/keymap"
	"github.com/grovetools/skills/pkg/skills"
	tuimuxmsg "github.com/grovetools/tuimux/messages"
)

// newTestModel builds a minimal browser Model suitable for exercising the
// keybinding dispatch and rendering without a real service. It seeds one
// non-builtin (workspace) skill and one builtin skill.
func newTestModel(hosted bool) Model {
	keys := skillskeymap.NewBrowserKeyMap(nil)
	h := help.New(keys)
	return Model{
		keys:              keys,
		help:              &h,
		theme:             theme.DefaultTheme,
		sequence:          keymap.NewSequenceState(),
		hosted:            hosted,
		filterInput:       textinput.New(),
		injectPromptInput: textinput.New(),
		showAllSkills:     true, // so filteredNodes returns nodes verbatim
		nodes: []DisplayNode{
			{Name: "wsskill", Source: skills.SourceTypeProject, Path: "/tmp/wsskill"},
			{Name: "builtinskill", Source: skills.SourceTypeBuiltin},
			{Name: "othergroup", IsGroup: true},
			{Name: "wsskill2", Source: skills.SourceTypeProject, Path: "/tmp/wsskill2"},
		},
	}
}

func keyRune(r rune) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
}

// msgFromCmd executes a tea.Cmd and returns the produced message, or nil.
func msgFromCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

func TestHostedPreviewToggle(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0 // wsskill (non-builtin)

	// First v opens a non-focused side split.
	updated, cmd := m.handleKeyMsg(keyRune('v'))
	m = updated.(Model)
	if !m.previewOpen {
		t.Fatal("expected previewOpen to be true after first v")
	}
	msg := msgFromCmd(cmd)
	open, ok := msg.(embed.SplitEditorRequestMsg)
	if !ok {
		t.Fatalf("expected SplitEditorRequestMsg, got %T", msg)
	}
	if open.Focus {
		t.Error("expected Focus to be false so the tree keeps focus")
	}
	// An even split: the tree narrows itself to fit, the editor cannot.
	if open.Ratio != 0.5 {
		t.Errorf("expected Ratio 0.5, got %v", open.Ratio)
	}
	if !strings.HasSuffix(open.Path, "/SKILL.md") {
		t.Errorf("expected path ending in SKILL.md, got %q", open.Path)
	}

	// Second v on the same skill closes the split.
	updated, cmd = m.handleKeyMsg(keyRune('v'))
	m = updated.(Model)
	if m.previewOpen {
		t.Fatal("expected previewOpen to be false after second v")
	}
	if _, ok := msgFromCmd(cmd).(embed.SplitEditorCloseRequestMsg); !ok {
		t.Fatalf("expected SplitEditorCloseRequestMsg on second v, got %T", msgFromCmd(cmd))
	}
}

func TestHostedEditOpensRailEditor(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0 // wsskill (non-builtin)

	// enter / space / edit must emit a rail EditRequestMsg, NOT a
	// SplitEditorRequestMsg (that is v's job).
	for _, msgIn := range []tea.KeyMsg{keyRune('e'), keyRune(' '), {Type: tea.KeyEnter}} {
		_, cmd := m.handleKeyMsg(msgIn)
		out := msgFromCmd(cmd)
		if _, ok := out.(embed.EditRequestMsg); !ok {
			t.Fatalf("key %v: expected EditRequestMsg, got %T", msgIn, out)
		}
		if _, ok := out.(embed.SplitEditorRequestMsg); ok {
			t.Fatalf("key %v: edit should not emit SplitEditorRequestMsg", msgIn)
		}
	}
}

// TestHostedOpenTargets pins the three-way split the skills browser shares
// with nb and flow: space/enter pin SKILL.md to its own treemux pane
// (Dedicated), e replaces the buffer in the host's singleton Editor pane.
func TestHostedOpenTargets(t *testing.T) {
	cases := []struct {
		name          string
		key           tea.KeyMsg
		wantDedicated bool
	}{
		{"space opens a dedicated pane", keyRune(' '), true},
		{"enter opens a dedicated pane", tea.KeyMsg{Type: tea.KeyEnter}, true},
		{"e quick-opens in the singleton editor", keyRune('e'), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(true)
			m.cursor = 0 // wsskill (non-builtin)
			_, cmd := m.handleKeyMsg(tc.key)
			out := msgFromCmd(cmd)
			req, ok := out.(embed.EditRequestMsg)
			if !ok {
				t.Fatalf("expected EditRequestMsg, got %T", out)
			}
			if req.Dedicated != tc.wantDedicated {
				t.Errorf("Dedicated = %v, want %v", req.Dedicated, tc.wantDedicated)
			}
			if !strings.HasSuffix(req.Path, "/SKILL.md") {
				t.Errorf("expected path ending in SKILL.md, got %q", req.Path)
			}
		})
	}
}

func TestBuiltinSkillCannotEditOrPreview(t *testing.T) {
	for _, key := range []tea.KeyMsg{keyRune('v'), keyRune('e'), {Type: tea.KeyEnter}} {
		m := newTestModel(true)
		m.cursor = 1 // builtinskill
		updated, cmd := m.handleKeyMsg(key)
		m = updated.(Model)
		if msg := msgFromCmd(cmd); msg != nil {
			t.Fatalf("key %v: builtin skill should emit no editor message, got %T", key, msg)
		}
		if !strings.Contains(strings.ToLower(m.statusMsg), "cannot") {
			t.Fatalf("key %v: expected a 'cannot' status, got %q", key, m.statusMsg)
		}
		if m.previewOpen {
			t.Fatalf("key %v: builtin skill must not open preview", key)
		}
	}
}

// openPreview presses v on the currently selected row and asserts the split
// actually opened, returning the resulting model.
func openPreview(t *testing.T, m Model) Model {
	t.Helper()
	updated, cmd := m.handleKeyMsg(keyRune('v'))
	m = updated.(Model)
	if !m.previewOpen {
		t.Fatal("setup: expected v to open the preview")
	}
	if _, ok := msgFromCmd(cmd).(embed.SplitEditorRequestMsg); !ok {
		t.Fatalf("setup: expected SplitEditorRequestMsg from v, got %T", msgFromCmd(cmd))
	}
	return m
}

// TestStickyPreviewFollowsCursor is the core sticky-navigation guard: while the
// preview split is open, moving onto another previewable skill must re-emit a
// SplitEditorRequestMsg for THAT skill with Ratio 0, so the host retargets the
// existing split in place instead of respawning it at a fresh geometry.
func TestStickyPreviewFollowsCursor(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0 // wsskill
	m = openPreview(t, m)

	// Jump to the other previewable skill (index 3); index 1 is a builtin and
	// index 2 is a group header, both covered separately below.
	m.cursor = 2
	updated, cmd := m.handleKeyMsg(keyRune('j'))
	m = updated.(Model)
	if !m.previewOpen {
		t.Error("cursor movement must not clear previewOpen — the split is still up")
	}
	req, ok := msgFromCmd(cmd).(embed.SplitEditorRequestMsg)
	if !ok {
		t.Fatalf("expected a sticky SplitEditorRequestMsg on cursor move, got %T", msgFromCmd(cmd))
	}
	if req.Ratio != 0 {
		t.Errorf("sticky re-emit must send Ratio 0 (preserve geometry), got %v", req.Ratio)
	}
	if req.Focus {
		t.Error("sticky re-emit must not steal focus from the tree")
	}
	if !strings.HasPrefix(req.Path, "/tmp/wsskill2/") {
		t.Errorf("preview should have followed to wsskill2, got %q", req.Path)
	}
}

// TestStickyPreviewSilentWhenClosed guards the other half: with no preview
// open, cursor movement must stay silent rather than opening one.
func TestStickyPreviewSilentWhenClosed(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0
	_, cmd := m.handleKeyMsg(keyRune('j'))
	if msg := msgFromCmd(cmd); msg != nil {
		t.Fatalf("cursor movement with no preview open must emit nothing, got %T", msg)
	}
}

// TestStickyPreviewParksOnUnpreviewableRows pins the deliberate choice for rows
// with no SKILL.md behind them: the split stays on the last previewable skill
// rather than closing, matching what v itself does on a builtin.
func TestStickyPreviewParksOnUnpreviewableRows(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cursor int
	}{
		{"builtin skill", 1},
		{"group header", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(true)
			m.cursor = 0
			m = openPreview(t, m)

			m.cursor = tc.cursor - 1
			updated, cmd := m.handleKeyMsg(keyRune('j'))
			m = updated.(Model)
			if m.cursor != tc.cursor {
				t.Fatalf("setup: cursor = %d, want %d", m.cursor, tc.cursor)
			}
			if msg := msgFromCmd(cmd); msg != nil {
				t.Errorf("expected no message for an unpreviewable row, got %T", msg)
			}
			if !m.previewOpen {
				t.Error("preview must stay open (parked) on an unpreviewable row")
			}
			if !strings.HasPrefix(m.previewPath, "/tmp/wsskill/") {
				t.Errorf("preview should still be parked on wsskill, got %q", m.previewPath)
			}

			// v here refuses without touching the open split, so the toggle
			// must still read "open" and close on the next v over a skill.
			updated, cmd = m.handleKeyMsg(keyRune('v'))
			m = updated.(Model)
			if !m.previewOpen {
				t.Error("v on an unpreviewable row must not clear previewOpen")
			}
			if msg := msgFromCmd(cmd); msg != nil {
				t.Errorf("v on an unpreviewable row must emit nothing, got %T", msg)
			}
		})
	}
}

// TestStickyPreviewSkipsRedundantReemit ensures a cursor move that lands back
// on the skill already in the split does not churn the host with a request.
func TestStickyPreviewSkipsRedundantReemit(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0
	m = openPreview(t, m)

	// k at the top of the list is a no-op move; so is re-selecting the same row.
	_, cmd := m.handleKeyMsg(keyRune('k'))
	if msg := msgFromCmd(cmd); msg != nil {
		t.Errorf("no-op cursor move must not re-emit, got %T", msg)
	}

	// A selection re-render that lands on the skill already in the split
	// (a list reload that didn't move the cursor) is a no-op for the host.
	if cmd := m.selectionChanged(); cmd != nil {
		t.Errorf("re-selecting the skill already previewed must not re-emit, got %T", msgFromCmd(cmd))
	}
}

// TestHostClosedSplitResyncsToggle covers the state-honesty case: when the user
// closes the preview from the host side (leader-x, or the editor exiting),
// treemux notifies the origin panel with DetailPaneClosedMsg. If skills ignored
// it, previewOpen would stay true and the next v would emit a close for a pane
// that no longer exists instead of reopening.
func TestHostClosedSplitResyncsToggle(t *testing.T) {
	for _, closeMsg := range []tea.Msg{
		tuimuxmsg.DetailPaneClosedMsg{DetailPanelID: "split-editor-1-SKILL.md", OriginPanelID: "skills-1"},
		embed.SplitEditorClosedMsg{Path: "/tmp/wsskill/SKILL.md"},
	} {
		m := newTestModel(true)
		m.cursor = 0
		m = openPreview(t, m)

		updated, _ := m.Update(closeMsg)
		m = updated.(Model)
		if m.previewOpen {
			t.Fatalf("%T: previewOpen must be false once the host closed the split", closeMsg)
		}
		if m.previewPath != "" {
			t.Errorf("%T: previewPath must be cleared, got %q", closeMsg, m.previewPath)
		}

		// Sticky navigation must go quiet, and v must OPEN rather than close.
		_, cmd := m.handleKeyMsg(keyRune('j'))
		if msg := msgFromCmd(cmd); msg != nil {
			t.Errorf("%T: sticky nav must stop after the split is gone, got %T", closeMsg, msg)
		}
		updated, cmd = m.handleKeyMsg(keyRune('v'))
		m = updated.(Model)
		if !m.previewOpen {
			t.Errorf("%T: v must reopen after a host-side close", closeMsg)
		}
		if _, ok := msgFromCmd(cmd).(embed.SplitEditorRequestMsg); !ok {
			t.Errorf("%T: v must emit an open request, got %T", closeMsg, msgFromCmd(cmd))
		}
	}
}

func TestStandalonePreviewFallsBackToEdit(t *testing.T) {
	m := newTestModel(false) // standalone
	m.cursor = 0
	_, cmd := m.handleKeyMsg(keyRune('v'))
	if cmd == nil {
		t.Fatal("expected a non-nil cmd (editSkillCmd fallback) in standalone mode")
	}
	if m.previewOpen {
		t.Error("standalone mode should not set previewOpen")
	}
}

// TestNarrowWidthRenderNoPanic ensures the two-pane layout survives very
// narrow widths without panicking on negative lipgloss widths.
func TestNarrowWidthRenderNoPanic(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		m := newTestModel(false)
		m.loading = false
		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = updated.(Model)
		out := m.View()
		if out == "" {
			t.Errorf("width %d: View() produced empty output", w)
		}
	}
}

// maxLineWidth returns the widest visible (ANSI-aware) line in a rendered
// block, ignoring trailing whitespace which does not affect wrapping.
func maxLineWidth(s string) int {
	maxW := 0
	for _, line := range strings.Split(s, "\n") {
		if w := lipgloss.Width(strings.TrimRight(line, " ")); w > maxW {
			maxW = w
		}
	}
	return maxW
}

func lineCount(s string) int { return len(strings.Split(s, "\n")) }

// TestDetailPaneTruncatesInsteadOfWrapping is the core regression guard for the
// ticket: at narrow widths the detail pane must TRUNCATE long lines (with an
// ellipsis) rather than soft-wrap them into many reflowed lines. We render the
// pane once with a short preview line and once with a pathologically long
// single line; truncation adds at most one output line, whereas the old
// width-wrapping reflow ballooned a single line into ~a dozen.
func TestDetailPaneTruncatesInsteadOfWrapping(t *testing.T) {
	for _, w := range []int{40, 60} {
		base := newTestModel(false)
		base.loading = false
		updated, _ := base.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		base = updated.(Model)
		base.cursor = 0
		contentWidth := base.viewport.Width - 2

		short := base
		short.cachedContent = "short preview line"
		shortOut := short.renderSkillDetails(&short.nodes[0])

		long := base
		long.cachedContent = strings.Repeat("x", contentWidth*12) // would wrap to ~12 lines
		longOut := long.renderSkillDetails(&long.nodes[0])

		if delta := lineCount(longOut) - lineCount(shortOut); delta > 1 {
			t.Errorf("width %d: a single long preview line added %d rendered lines (want <=1); it is wrapping, not truncating", w, delta)
		}
		if !strings.Contains(longOut, "…") {
			t.Errorf("width %d: expected a truncation ellipsis in the detail pane", w)
		}
		if got := maxLineWidth(longOut); got > contentWidth {
			t.Errorf("width %d: detail line width %d exceeds content width %d", w, got, contentWidth)
		}
	}
}

// TestNarrowWidthNoLineExceedsPanel renders the full browser (header + two
// panes) and the footer at narrow widths with long content and asserts no
// rendered line exceeds the panel width, so nothing overflows the terminal and
// forces a hard wrap. This guards the header/footer/left-pane paths, which are
// joined with JoinVertical and are not otherwise width-bounded.
func TestNarrowWidthNoLineExceedsPanel(t *testing.T) {
	longDesc := strings.Repeat("verylongtoken ", 60)
	longPath := "/very/long/absolute/path/" + strings.Repeat("segment/", 40) + "SKILL.md"

	for _, w := range []int{20, 40, 60} {
		m := newTestModel(false)
		m.loading = false
		m.nodes[0].Description = longDesc
		m.nodes[0].Path = longPath
		m.nodes[0].Workspace = strings.Repeat("ws", 40)

		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = updated.(Model)
		m.cursor = 0
		// Exercise the search footer path (long filter text) and a long preview.
		m.searching = true
		m.filterInput.SetValue(strings.Repeat("query", 20))
		m.cachedContent = strings.Repeat("x", 500) + "\n" + longDesc
		m.viewport.SetContent(m.renderSkillDetails(&m.nodes[0]))

		if got := maxLineWidth(m.View()); got > w {
			t.Errorf("width %d: View() has a line of width %d (must be <= %d)", w, got, w)
		}
		if got := maxLineWidth(m.FooterView()); got > w {
			t.Errorf("width %d: FooterView() has a line of width %d (must be <= %d)", w, got, w)
		}

		// Group details path.
		m.nodes = append([]DisplayNode{{Name: strings.Repeat("group", 20), IsGroup: true}}, m.nodes...)
		m.viewport.SetContent(m.renderGroupDetails(&m.nodes[0]))
		if got := maxLineWidth(m.View()); got > w {
			t.Errorf("width %d: group View() has a line of width %d (must be <= %d)", w, got, w)
		}
	}
}

// sendChord types a multi-key sequence one rune at a time through Update,
// which is where the browser's SequenceState lives. handleKeyMsg is downstream
// of the matcher and never sees a chord, so chord tests must go through Update.
func sendChord(m Model, keys string) (Model, tea.Msg) {
	var out tea.Msg
	for _, r := range keys {
		updated, cmd := m.Update(keyRune(r))
		m = updated.(Model)
		if msg := msgFromCmd(cmd); msg != nil {
			out = msg
		}
	}
	return m, out
}

// enterInject types the sa chord and asserts the browser actually entered
// inject mode, returning the resulting model.
func enterInject(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = sendChord(m, "sa")
	if !m.injectMode {
		t.Fatal("setup: expected sa to enter inject mode")
	}
	return m
}

// pressInject sends a key through handleKeyMsg and returns the new model plus
// whatever message the resulting cmd produced.
func pressInject(m Model, msg tea.KeyMsg) (Model, tea.Msg) {
	updated, cmd := m.handleKeyMsg(msg)
	return updated.(Model), msgFromCmd(cmd)
}

// TestInjectModeRequiresHost pins the two answers to sa: hosted it opens the
// mode, standalone it explains why it can't rather than stranding the user in
// a mode whose enter key could never reach an agent.
func TestInjectModeRequiresHost(t *testing.T) {
	hostedModel, out := sendChord(newTestModel(true), "sa")
	if !hostedModel.injectMode {
		t.Error("hosted: sa must enter inject mode")
	}
	if out != nil {
		t.Errorf("hosted: entering inject mode must emit nothing, got %T", out)
	}

	standalone, _ := sendChord(newTestModel(false), "sa")
	if standalone.injectMode {
		t.Error("standalone: sa must not enter inject mode")
	}
	if !strings.Contains(standalone.statusMsg, "treemux-hosted") {
		t.Errorf("standalone: expected an explanatory status, got %q", standalone.statusMsg)
	}
}

// TestInjectChordFallsThrough is the guard for the case most likely to strand a
// user: the sequence state has no timeout, so a lone "s" arms the chord and
// waits indefinitely. Whatever arrives next that is not "a" must clear the
// buffer AND be handled on its own terms — swallowing it, or leaving the buffer
// armed so the key after that gets eaten too, would make the browser feel dead.
func TestInjectChordFallsThrough(t *testing.T) {
	for _, tc := range []struct {
		name  string
		after rune
		check func(t *testing.T, m Model)
	}{
		{"s then j still moves the cursor", 'j', func(t *testing.T, m Model) {
			if m.cursor != 1 {
				t.Errorf("cursor = %d, want 1 — the fall-through key was swallowed", m.cursor)
			}
		}},
		{"s then / still opens search", '/', func(t *testing.T, m Model) {
			if !m.searching {
				t.Error("/ after a pending s must still open search")
			}
		}},
		// Double-tapping the prefix disarms rather than re-arming: "ss" is not a
		// binding and not a prefix of one, so the matcher clears and the second
		// s falls through to a keymap that no longer binds it (sync moved to S)
		// — a dead keystroke, not a wedge. The following "a" is likewise inert,
		// so a fumbled "ssa" costs the user a retry and nothing else. Pinned
		// because the alternative worth having (re-offering the stray key to
		// the matcher) would need a second dispatch path, and the plain gg
		// pattern is the one this browser follows.
		{"s then s disarms without wedging", 's', func(t *testing.T, m Model) {
			if m.sequence.IsPending() {
				t.Errorf("a dead chord must not leave a buffer armed, got %q", m.sequence.Buffer())
			}
			m2, _ := sendChord(m, "sa")
			if !m2.injectMode {
				t.Error("sa must work immediately after a fumbled double-s")
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(true)
			m.cursor = 0

			updated, _ := m.Update(keyRune('s'))
			m = updated.(Model)
			if !m.sequence.IsPending() {
				t.Fatal("setup: a lone s must arm the chord")
			}
			if m.injectMode {
				t.Fatal("a lone s must not enter inject mode")
			}

			updated, _ = m.Update(keyRune(tc.after))
			m = updated.(Model)
			if m.injectMode {
				t.Errorf("s then %q must not enter inject mode", tc.after)
			}
			tc.check(t, m)
		})
	}

	// And the buffer must not survive a completed chord either.
	m := enterInject(t, newTestModel(true))
	if m.sequence.IsPending() {
		t.Errorf("a matched chord must clear the buffer, got %q", m.sequence.Buffer())
	}
}

// TestSyncMovedOffLowercaseS pins the rebinding that made the chord possible.
// Lowercase s is now nothing but a prefix; sync answers to S.
func TestSyncMovedOffLowercaseS(t *testing.T) {
	m := newTestModel(true)

	updated, cmd := m.Update(keyRune('S'))
	if got := updated.(Model).statusMsg; got != "Syncing..." {
		t.Errorf("S: statusMsg = %q, want %q", got, "Syncing...")
	}
	if _, ok := msgFromCmd(cmd).(syncCompleteMsg); !ok {
		t.Errorf("S must trigger a sync, got %T", msgFromCmd(cmd))
	}

	// A lone s arms the chord and syncs nothing.
	updated, cmd = m.Update(keyRune('s'))
	lower := updated.(Model)
	if lower.statusMsg == "Syncing..." {
		t.Error("lowercase s must no longer sync — it is the sa prefix")
	}
	if msg := msgFromCmd(cmd); msg != nil {
		t.Errorf("a pending prefix must emit nothing, got %T", msg)
	}
}

// TestInjectSelectionOrderPreserved is the core selection guard: the batch is
// an ordered list, not a set, because its order is the order the host types
// the lines at the agent's prompt.
func TestInjectSelectionOrderPreserved(t *testing.T) {
	m := enterInject(t, newTestModel(true))

	// Select wsskill2 (index 3) first, then wsskill (index 0).
	m.cursor = 3
	m, _ = pressInject(m, keyRune(' '))
	m.cursor = 0
	m, _ = pressInject(m, keyRune(' '))
	if got, want := m.injectSelected, []string{"wsskill2", "wsskill"}; !equalStrings(got, want) {
		t.Fatalf("selection = %v, want %v", got, want)
	}

	// Deselecting the first pick and re-selecting it must move it to the END:
	// re-picking is how a user rebuilds the send order.
	m.cursor = 3
	m, _ = pressInject(m, keyRune(' '))
	if got, want := m.injectSelected, []string{"wsskill"}; !equalStrings(got, want) {
		t.Fatalf("after deselect: selection = %v, want %v", got, want)
	}
	m, _ = pressInject(m, keyRune(' '))
	if got, want := m.injectSelected, []string{"wsskill", "wsskill2"}; !equalStrings(got, want) {
		t.Fatalf("after reselect: selection = %v, want %v", got, want)
	}
}

// TestInjectSelectionRefusesUnsendableRows covers the two rows that have no
// "/name" behind them: builtins (embedded, never installed for the agent) and
// group headers (display scaffolding).
func TestInjectSelectionRefusesUnsendableRows(t *testing.T) {
	for _, tc := range []struct {
		name       string
		cursor     int
		wantStatus string
	}{
		{"builtin skill", 1, "builtin"},
		{"group header", 2, "Groups"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := enterInject(t, newTestModel(true))
			m.cursor = tc.cursor
			m, _ = pressInject(m, keyRune(' '))
			if len(m.injectSelected) != 0 {
				t.Errorf("selection = %v, want empty", m.injectSelected)
			}
			if !strings.Contains(m.statusMsg, tc.wantStatus) {
				t.Errorf("status = %q, want it to mention %q", m.statusMsg, tc.wantStatus)
			}
		})
	}
}

// TestSpaceOutsideInjectModeStillOpensPane is the regression guard for the
// overload: space only means "select" inside the mode. Everywhere else it must
// still pin SKILL.md to its own treemux pane.
func TestSpaceOutsideInjectModeStillOpensPane(t *testing.T) {
	m := newTestModel(true)
	m.cursor = 0
	_, out := pressInject(m, keyRune(' '))
	req, ok := out.(embed.EditRequestMsg)
	if !ok {
		t.Fatalf("expected EditRequestMsg outside inject mode, got %T", out)
	}
	if !req.Dedicated {
		t.Error("space outside inject mode must still request a dedicated pane")
	}
}

// TestInjectNavigationStillWorks pins the overlay contract: keys the mode does
// not claim fall through to the normal keymap, so the tree still navigates and
// filters while a batch is being assembled.
func TestInjectNavigationStillWorks(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m, _ = pressInject(m, keyRune('j'))
	if m.cursor != 1 {
		t.Errorf("j must still move the cursor in inject mode, cursor = %d", m.cursor)
	}
	m, _ = pressInject(m, keyRune('/'))
	if !m.searching {
		t.Error("/ must still open search in inject mode")
	}
	if !m.injectMode {
		t.Error("navigation keys must not drop out of inject mode")
	}
}

// TestInjectPromptAttachesInstruction walks the p flow end to end: open the
// editor, type, commit, dispatch — and assert the line the host receives is
// "/name instruction".
func TestInjectPromptAttachesInstruction(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.cursor = 0

	// p selects implicitly and opens the editor.
	m, _ = pressInject(m, keyRune('p'))
	if !m.injectPromptEditing {
		t.Fatal("p must open the prompt editor")
	}
	if m.injectPromptTarget != "wsskill" {
		t.Errorf("prompt target = %q, want wsskill", m.injectPromptTarget)
	}
	if got, want := m.injectSelected, []string{"wsskill"}; !equalStrings(got, want) {
		t.Errorf("p must select implicitly: selection = %v, want %v", got, want)
	}

	// The editor owns every key, so typing routes through Update.
	for _, r := range "be brief" {
		updated, _ := m.Update(keyRune(r))
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.injectPromptEditing {
		t.Fatal("Enter must close the prompt editor")
	}
	if !m.injectMode {
		t.Fatal("committing a prompt must not leave inject mode")
	}
	if got := m.injectPrompts["wsskill"]; got != "be brief" {
		t.Fatalf("stored prompt = %q, want %q", got, "be brief")
	}

	_, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEnter})
	req, ok := out.(embed.AgentInjectRequestMsg)
	if !ok {
		t.Fatalf("expected AgentInjectRequestMsg, got %T", out)
	}
	if got, want := req.Lines, []string{"/wsskill be brief"}; !equalStrings(got, want) {
		t.Errorf("lines = %v, want %v", got, want)
	}
}

// TestInjectPromptEscKeepsBatch pins the narrow scope of Esc inside the prompt
// editor: it abandons the edit, not the mode and not the selection.
func TestInjectPromptEscKeepsBatch(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.cursor = 0
	m, _ = pressInject(m, keyRune('p'))
	for _, r := range "oops" {
		updated, _ := m.Update(keyRune(r))
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(Model)

	if m.injectPromptEditing {
		t.Error("Esc must close the prompt editor")
	}
	if !m.injectMode {
		t.Error("Esc in the prompt editor must not leave inject mode")
	}
	if got, want := m.injectSelected, []string{"wsskill"}; !equalStrings(got, want) {
		t.Errorf("selection = %v, want %v", got, want)
	}
	if got, ok := m.injectPrompts["wsskill"]; ok {
		t.Errorf("cancelled edit must store nothing, got %q", got)
	}
}

// TestInjectDispatch is the payload guard: enter sends one line per selected
// skill, in selection order, asks for focus, and leaves the mode clean.
func TestInjectDispatch(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.cursor = 3
	m, _ = pressInject(m, keyRune(' '))
	m.cursor = 0
	m, _ = pressInject(m, keyRune(' '))

	m, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEnter})
	req, ok := out.(embed.AgentInjectRequestMsg)
	if !ok {
		t.Fatalf("expected AgentInjectRequestMsg, got %T", out)
	}
	if got, want := req.Lines, []string{"/wsskill2", "/wsskill"}; !equalStrings(got, want) {
		t.Errorf("lines = %v, want %v (selection order)", got, want)
	}
	if !req.Focus {
		t.Error("dispatch must ask the host to focus the agent pane")
	}
	if m.injectMode {
		t.Error("dispatch must leave inject mode")
	}
	if len(m.injectSelected) != 0 || len(m.injectPrompts) != 0 {
		t.Errorf("dispatch must clear the batch, got %v / %v", m.injectSelected, m.injectPrompts)
	}
	if !m.injectPending {
		t.Error("dispatch must arm injectPending so the result is accepted")
	}
}

// TestInjectDispatchWithEmptySelectionStays guards the impatient-enter case:
// nothing is sent and the mode survives, so the user keeps the prompt edits
// they were about to make.
func TestInjectDispatchWithEmptySelectionStays(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEnter})
	if out != nil {
		t.Fatalf("empty selection must emit nothing, got %T", out)
	}
	if !m.injectMode {
		t.Error("empty enter must stay in inject mode")
	}
	if m.injectPending {
		t.Error("empty enter must not arm injectPending")
	}
	if m.statusMsg == "" {
		t.Error("empty enter should explain that nothing is selected")
	}
}

// withBatch enters inject mode and assembles a one-skill batch carrying an
// instruction, so a caller can assert what survives (or doesn't).
func withBatch(t *testing.T, m Model) Model {
	t.Helper()
	m = enterInject(t, m)
	m.cursor = 0
	m, _ = pressInject(m, keyRune('p'))
	updated, _ := m.Update(keyRune('x'))
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if len(m.injectSelected) != 1 || len(m.injectPrompts) != 1 {
		t.Fatalf("setup: want a 1-skill batch with 1 prompt, got %v / %v", m.injectSelected, m.injectPrompts)
	}
	return m
}

// TestInjectCancelClearsBatch covers the cancel key. Esc is now the ONLY one:
// the old "a second I toggles off" path went away with the I binding, and sa
// deliberately did not inherit it (see below).
func TestInjectCancelClearsBatch(t *testing.T) {
	m := withBatch(t, newTestModel(true))

	m, out := pressInject(m, tea.KeyMsg{Type: tea.KeyEscape})
	if out != nil {
		t.Errorf("cancel must emit nothing, got %T", out)
	}
	if m.injectMode {
		t.Error("cancel must leave inject mode")
	}
	if len(m.injectSelected) != 0 {
		t.Errorf("cancel must clear the selection, got %v", m.injectSelected)
	}
	if len(m.injectPrompts) != 0 {
		t.Errorf("cancel must clear the prompts, got %v", m.injectPrompts)
	}
}

// TestInjectChordInsideModeIsNonDestructive pins the deliberate replacement for
// the old I toggle. sa is now cheap enough to fat-finger mid-batch, and
// enterInjectMode always starts fresh, so re-entering would silently destroy
// work. It is a no-op instead; esc is the way out.
func TestInjectChordInsideModeIsNonDestructive(t *testing.T) {
	m := withBatch(t, newTestModel(true))
	before := append([]string(nil), m.injectSelected...)

	m, out := sendChord(m, "sa")
	if out != nil {
		t.Errorf("sa inside the mode must emit nothing, got %T", out)
	}
	if !m.injectMode {
		t.Error("sa inside the mode must not toggle it off — esc is the cancel")
	}
	if !equalStrings(m.injectSelected, before) {
		t.Errorf("sa inside the mode must not touch the batch: %v, want %v", m.injectSelected, before)
	}
	if len(m.injectPrompts) != 1 {
		t.Errorf("sa inside the mode must not drop attached prompts, got %v", m.injectPrompts)
	}
}

// TestInjectResultIsGatedByPending is the broadcast guard: the host fans the
// result out to every panel, so a browser that never dispatched must ignore it
// rather than reporting a neighbour's failure as its own.
func TestInjectResultIsGatedByPending(t *testing.T) {
	failure := embed.AgentInjectResultMsg{Err: "no agent pane in scope"}

	idle := newTestModel(true)
	updated, _ := idle.Update(failure)
	idle = updated.(Model)
	if idle.errorMsg != "" {
		t.Errorf("a browser with no pending request must ignore the result, got errorMsg %q", idle.errorMsg)
	}

	pending := newTestModel(true)
	pending.injectPending = true
	updated, _ = pending.Update(failure)
	pending = updated.(Model)
	if !strings.Contains(pending.errorMsg, "no agent pane in scope") {
		t.Errorf("errorMsg = %q, want it to carry the host's error", pending.errorMsg)
	}
	if pending.injectPending {
		t.Error("the first result must clear injectPending")
	}

	// A second, unrelated broadcast must no longer be adopted.
	pending.errorMsg = ""
	updated, _ = pending.Update(embed.AgentInjectResultMsg{Err: "someone else's problem"})
	pending = updated.(Model)
	if pending.errorMsg != "" {
		t.Errorf("a post-result broadcast must be ignored, got %q", pending.errorMsg)
	}
}

// TestInjectResultSuccessStatus pins the success wording, including the
// pluralisation and the target suffix.
func TestInjectResultSuccessStatus(t *testing.T) {
	for _, tc := range []struct {
		result embed.AgentInjectResultMsg
		want   string
	}{
		{embed.AgentInjectResultMsg{Delivered: 1, Target: "claude"}, "Injected 1 skill → claude"},
		{embed.AgentInjectResultMsg{Delivered: 2, Target: "claude"}, "Injected 2 skills → claude"},
		{embed.AgentInjectResultMsg{Delivered: 2}, "Injected 2 skills"},
	} {
		m := newTestModel(true)
		m.injectPending = true
		updated, _ := m.Update(tc.result)
		m = updated.(Model)
		if m.statusMsg != tc.want {
			t.Errorf("statusMsg = %q, want %q", m.statusMsg, tc.want)
		}
		if m.errorMsg != "" {
			t.Errorf("a successful result must not set errorMsg, got %q", m.errorMsg)
		}
	}
}

// TestInjectModeRenderNoOverflow re-runs the narrow-width layout guard with the
// mode's extra markers in play, since the selection marker and the prompt
// editor both add width the base tests never see.
func TestInjectModeRenderNoOverflow(t *testing.T) {
	for _, w := range []int{20, 40, 80} {
		m := newTestModel(true)
		m.loading = false
		updated, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: 24})
		m = updated.(Model)
		m = enterInject(t, m)
		m.cursor = 0
		m, _ = pressInject(m, keyRune(' '))
		m, _ = pressInject(m, keyRune('p'))

		if got := maxLineWidth(m.View()); got > w {
			t.Errorf("width %d: inject View() has a line of width %d (must be <= %d)", w, got, w)
		}
		if got := maxLineWidth(m.FooterView()); got > w {
			t.Errorf("width %d: inject FooterView() has a line of width %d (must be <= %d)", w, got, w)
		}
	}
}

// TestInjectModeFooterKeepsItsRowBudget is the regression guard for the visible
// jump on entering the mode: the pager reserves one footer row and re-derives
// the body height from the rendered footer, so a footer that grows steals a row
// from the tree. Plain inject mode must therefore cost exactly what normal mode
// costs. The prompt editor is allowed the one extra line — that is the same
// budget the search input has always spent, and only while it is open.
func TestInjectModeFooterKeepsItsRowBudget(t *testing.T) {
	base := newTestModel(true)
	base.loading = false
	updated, _ := base.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	base = updated.(Model)

	normal := lineCount(base.FooterView())
	if normal != 1 {
		t.Fatalf("setup: normal-mode footer = %d lines, want 1 (pager reserves one row)", normal)
	}

	inj := enterInject(t, base)
	if got := lineCount(inj.FooterView()); got != normal {
		t.Errorf("inject-mode footer = %d lines, want %d — an extra row shifts the whole panel", got, normal)
	}

	// A selection and its status feedback must not grow it either.
	inj.cursor = 0
	inj, _ = pressInject(inj, keyRune(' '))
	if got := lineCount(inj.FooterView()); got != normal {
		t.Errorf("inject-mode footer after a selection = %d lines, want %d", got, normal)
	}

	// The editor is the one sanctioned extra row, and only while open.
	editing, _ := pressInject(inj, keyRune('p'))
	if got := lineCount(editing.FooterView()); got != normal+1 {
		t.Errorf("prompt-editor footer = %d lines, want %d (the editor line, like search)", got, normal+1)
	}
	closed, _ := editing.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if got := lineCount(closed.(Model).FooterView()); got != normal {
		t.Errorf("footer after closing the editor = %d lines, want %d", got, normal)
	}
}

// TestFooterHelpIsQuitAndHelpOnly encodes the user-approved trim: the footer
// carries q and ?, and nothing else, in EVERY mode. The full keymap stays
// discoverable through the ? overlay (FullHelp/Sections), which is the point —
// so this asserts the overlay still lists what the footer dropped.
func TestFooterHelpIsQuitAndHelpOnly(t *testing.T) {
	base := newTestModel(true)
	base.loading = false
	updated, _ := base.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	base = updated.(Model)

	for _, tc := range []struct {
		name string
		m    Model
	}{
		{"normal", base},
		{"inject", enterInject(t, base)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			footer := ansi.Strip(tc.m.FooterView())
			if !strings.Contains(footer, "Press ? for help") {
				t.Errorf("footer must keep the ? affordance, got %q", footer)
			}
			if !strings.Contains(footer, "quit") {
				t.Errorf("footer must keep quit, got %q", footer)
			}
			for _, gone := range []string{"open in pane", "edit", "preview"} {
				if strings.Contains(footer, gone) {
					t.Errorf("footer must no longer advertise %q, got %q", gone, footer)
				}
			}
		})
	}

	// The trim is a footer trim only — ? must still reach everything.
	var full []string
	for _, section := range base.keys.Sections() {
		for _, b := range section.Bindings {
			full = append(full, b.Help().Desc)
		}
	}
	joined := strings.Join(full, "|")
	for _, want := range []string{"open in pane", "send to agent", "sync"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Sections() must still list %q, got %q", want, joined)
		}
	}
}

// TestInjectHintLivesInHeader pins where the key hint went. The header is a
// fixed two rows and is width-truncated, so it can carry the hint for free;
// the footer cannot.
func TestInjectHintLivesInHeader(t *testing.T) {
	m := enterInject(t, newTestModel(true))
	m.width = 200 // wide enough that nothing is truncated away

	header := ansi.Strip(m.renderHeader(m.width))
	for _, want := range []string{"INJECT", "space select", "p prompt", "enter send", "esc cancel"} {
		if !strings.Contains(header, want) {
			t.Errorf("header must carry %q, got %q", want, header)
		}
	}
	if got := lineCount(m.renderHeader(m.width)); got != 2 {
		t.Errorf("header = %d lines, want 2 (title + separator)", got)
	}
	if footer := ansi.Strip(m.FooterView()); strings.Contains(footer, "space select") {
		t.Errorf("the hint must not also sit in the footer, got %q", footer)
	}
}

// TestInjectHeaderTruncatesAtNarrowWidths keeps the longer banner inside the
// panel: it is the widest thing the header ever renders.
func TestInjectHeaderTruncatesAtNarrowWidths(t *testing.T) {
	for _, w := range []int{20, 40, 60} {
		m := enterInject(t, newTestModel(true))
		m.width = w
		if got := maxLineWidth(m.renderHeader(w)); got > w {
			t.Errorf("width %d: inject header line width %d exceeds the panel", w, got)
		}
	}
}

// TestInjectMarkerAgreesWithReservation is the alignment guard. renderNode's
// marker and getLeftPaneWidth's reservation used to be two independent
// hardcoded 2s; they now both read injectMarkWidth(), and this asserts that in
// BOTH icon variants — the exact assumption the old "must be exactly 2 cells"
// comment made and could not keep.
func TestInjectMarkerAgreesWithReservation(t *testing.T) {
	t.Cleanup(func() { theme.SetIcons("nerd") })

	for _, icons := range []string{"nerd", "ascii"} {
		t.Run(icons, func(t *testing.T) {
			theme.SetIcons(icons)
			want := injectMarkWidth()

			m := enterInject(t, newTestModel(true))
			m.width = 120
			m.cursor = 0
			m, _ = pressInject(m, keyRune(' ')) // select wsskill, leave wsskill2 unpicked

			// Every row spends the same cells, picked or not, skill or group.
			for _, node := range m.nodes {
				if got := lipgloss.Width(m.injectMark(node)); got != want {
					t.Errorf("mark for %q = %d cells, want %d", node.Name, got, want)
				}
			}

			// The reservation is that same number, not a parallel constant.
			plain := m
			plain.injectMode = false
			if got := m.getLeftPaneWidth() - plain.getLeftPaneWidth(); got != want {
				t.Errorf("getLeftPaneWidth widened by %d, want %d", got, want)
			}

			// And the tree really does line up: the picked row's name starts in
			// the same display COLUMN as the unpicked row's. Columns, not byte
			// offsets — the nerd marker is four bytes and the ascii one three,
			// which is precisely the confusion this whole fix is about.
			picked := ansi.Strip(m.renderNode(m.nodes[0], false, 100))
			unpicked := ansi.Strip(m.renderNode(m.nodes[3], false, 100))
			if a, b := nameColumn(t, picked, "wsskill"), nameColumn(t, unpicked, "wsskill2"); a != b {
				t.Errorf("name column: picked at %d, unpicked at %d (rows %q / %q)", a, b, picked, unpicked)
			}
		})
	}
}

// nameColumn returns the display column a name starts at in an ANSI-stripped
// rendered row.
func nameColumn(t *testing.T, row, name string) int {
	t.Helper()
	i := strings.Index(row, name)
	if i < 0 {
		t.Fatalf("row %q does not contain %q", row, name)
	}
	return lipgloss.Width(row[:i])
}

// TestInjectPromptMarkerIsThemed pins the swap from the literal U+270E pencil
// (a hairline in most terminal fonts, and absent from the ascii set entirely)
// to the theme's speech bubble.
func TestInjectPromptMarkerIsThemed(t *testing.T) {
	t.Cleanup(func() { theme.SetIcons("nerd") })

	for _, icons := range []string{"nerd", "ascii"} {
		t.Run(icons, func(t *testing.T) {
			theme.SetIcons(icons)

			m := enterInject(t, newTestModel(true))
			m.cursor = 0
			m, _ = pressInject(m, keyRune('p'))
			for _, r := range "be brief" {
				updated, _ := m.Update(keyRune(r))
				m = updated.(Model)
			}
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = updated.(Model)

			withPrompt := ansi.Strip(m.renderNode(m.nodes[0], false, 100))
			if !strings.Contains(withPrompt, theme.IconChat) {
				t.Errorf("a row with an instruction must carry IconChat (%q), got %q", theme.IconChat, withPrompt)
			}
			if strings.Contains(withPrompt, "✎") {
				t.Errorf("the literal pencil must be gone, got %q", withPrompt)
			}

			// A picked row with no instruction stays bare.
			m.cursor = 3
			m, _ = pressInject(m, keyRune(' '))
			bare := ansi.Strip(m.renderNode(m.nodes[3], false, 100))
			if strings.Contains(bare, theme.IconChat) {
				t.Errorf("a row with no instruction must not carry IconChat, got %q", bare)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestGetLeftPaneWidthNeverExceedsPanel(t *testing.T) {
	for _, w := range []int{10, 20, 30, 40, 80, 120} {
		m := newTestModel(false)
		m.width = w
		lw := m.getLeftPaneWidth()
		if lw >= w {
			t.Errorf("width %d: left pane %d must be < total width", w, lw)
		}
		if lw < 1 {
			t.Errorf("width %d: left pane %d must be >= 1", w, lw)
		}
	}
}

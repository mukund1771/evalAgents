package tui

import (
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mukund1771/evalAgents/internal/eval"
)

// plainStyles binds every style to a non-terminal writer, which puts lipgloss on
// its Ascii profile so rendered text comes back with no escape sequences. That
// makes assertions on output exact, and it stops AdaptiveColor from issuing the
// OSC 11 background query, which otherwise waits on a five-second timeout when a
// terminal happens to be attached to the test run.
func plainStyles() styles { return newStyles(lipgloss.NewRenderer(io.Discard)) }

func span(step, start, end int) eval.Span {
	return eval.Span{StepIndex: step, Start: start, End: end}
}

// An offset arrives from a file a user can edit and, for the judge, straight from
// a model. parseJudge checks StepIndex and End >= Start but never that End is
// inside the content, so these are all reachable.
func TestSpanIntervalsRejectsOffsetsItCannotStandBehind(t *testing.T) {
	const content = "hello world" // 11 bytes

	for _, sp := range []eval.Span{
		span(0, -1, 4),    // negative start
		span(0, 0, 12),    // past the end
		span(0, 12, 14),   // wholly past the end
		span(0, 6, 3),     // inverted
		span(0, -5, 9999), // what a hallucinating judge looks like
	} {
		keep, bad := spanIntervals(content, []eval.Span{sp}, 0)
		if len(keep) != 0 || len(bad) != 1 {
			t.Fatalf("span %#v: keep=%#v bad=%#v, want it refused", sp, keep, bad)
		}
	}

	// Valid and empty is not an anomaly: stepSpan emits exactly this for a step
	// with no content, so reporting it would cry wolf on every tool call.
	keep, bad := spanIntervals("", []eval.Span{span(0, 0, 0)}, 0)
	if len(keep) != 0 || len(bad) != 0 {
		t.Fatalf("empty span: keep=%#v bad=%#v, want both empty", keep, bad)
	}

	// A span for another step is not this step's business.
	keep, bad = spanIntervals(content, []eval.Span{span(3, 0, 5)}, 0)
	if len(keep) != 0 || len(bad) != 0 {
		t.Fatalf("other step: keep=%#v bad=%#v", keep, bad)
	}
}

func TestSpanIntervalsSnapsToRuneBoundaries(t *testing.T) {
	const content = "héllo" // h=1 é=2 l l o
	// Start 1, End 2 lands inside the two bytes of é.
	keep, bad := spanIntervals(content, []eval.Span{span(0, 1, 2)}, 0)
	if len(bad) != 0 || len(keep) != 1 {
		t.Fatalf("keep=%#v bad=%#v", keep, bad)
	}
	iv := keep[0]
	if iv.lo != 1 || iv.hi != 3 {
		t.Fatalf("interval = %#v, want {1,3} to enclose the whole rune", iv)
	}
	if got := content[iv.lo:iv.hi]; got != "é" || !utf8.ValidString(got) {
		t.Fatalf("slice = %q, want a whole é", got)
	}
	// Widening must never collapse a span to nothing.
	if iv.lo >= iv.hi {
		t.Fatal("interval collapsed")
	}
}

func TestSpanIntervalsMergesOverlaps(t *testing.T) {
	const content = "abcdefghij"
	st := plainStyles()

	for _, tc := range []struct {
		name  string
		spans []eval.Span
		want  interval
	}{
		{"overlapping", []eval.Span{span(0, 0, 5), span(0, 3, 9)}, interval{0, 9}},
		{"touching", []eval.Span{span(0, 0, 3), span(0, 3, 6)}, interval{0, 6}},
		{"nested", []eval.Span{span(0, 0, 9), span(0, 2, 4)}, interval{0, 9}},
		{"reversed input", []eval.Span{span(0, 3, 9), span(0, 0, 5)}, interval{0, 9}},
	} {
		keep, _ := spanIntervals(content, tc.spans, 0)
		if len(keep) != 1 || keep[0] != tc.want {
			t.Fatalf("%s: keep=%#v, want one %#v", tc.name, keep, tc.want)
		}
		// Nested markers would render as [[a[[b]]c]] and be unreadable.
		out := renderSpanned(content, keep, 80, st)
		if n := strings.Count(out, evidenceOpen); n != 1 {
			t.Fatalf("%s: %d open markers in %q, want 1", tc.name, n, out)
		}
	}
}

// This is the test that actually establishes the safety claim, rather than a
// comment asserting it: every offset pair in and well outside the string, on
// content with multi-byte runes.
func TestRenderSpannedSurvivesEveryOffsetPair(t *testing.T) {
	const content = "a€b😀c\nd\te"
	st := plainStyles()
	n := len(content)
	tried := 0
	for start := -3; start <= n+3; start++ {
		for end := -3; end <= n+3; end++ {
			keep, _ := spanIntervals(content, []eval.Span{span(0, start, end)}, 0)
			out := renderSpanned(content, keep, 12, st)
			if !utf8.ValidString(out) {
				t.Fatalf("start=%d end=%d produced invalid UTF-8", start, end)
			}
			tried++
		}
	}
	if tried < 100 {
		t.Fatalf("only %d combinations tried", tried)
	}
}

// Trajectory content is agent output and this is the first thing to render it
// raw. An escape sequence in a final answer must not reach the terminal.
func TestRenderSpannedNeutralisesControlBytes(t *testing.T) {
	st := plainStyles()
	out := renderSpanned("before\x1b[2Jafter\rmore\x00end\x07", nil, 200, st)
	for _, bad := range []string{"\x1b", "\r", "\x00", "\x07"} {
		if strings.Contains(out, bad) {
			t.Fatalf("output still contains %q: %q", bad, out)
		}
	}
	// The printable text around them survives.
	for _, want := range []string{"before", "after", "more", "end"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lost %q: %q", want, out)
		}
	}

	// Invalid UTF-8 is neutralised too, so output is always renderable.
	out = renderSpanned("ok\xff\xfe", nil, 80, st)
	if !utf8.ValidString(out) {
		t.Fatalf("invalid input produced invalid output: %q", out)
	}
}

func TestRenderSpannedWrapsAndKeepsMarksOnEachLine(t *testing.T) {
	st := plainStyles()
	content := strings.Repeat("x", 120)
	keep, _ := spanIntervals(content, []eval.Span{span(0, 0, 120)}, 0)
	out := renderSpanned(content, keep, 20, st)

	lines := strings.Split(out, "\n")
	if len(lines) < 6 {
		t.Fatalf("120 chars at width 20 gave %d lines", len(lines))
	}
	for i, line := range lines {
		if w := lipgloss.Width(line); w > 20 {
			t.Fatalf("line %d is %d cells wide: %q", i, w, line)
		}
		// A span covering everything must be marked on every line it covers, or
		// the highlight silently disappears when content wraps.
		if !strings.Contains(line, evidenceOpen) || !strings.Contains(line, evidenceClose) {
			t.Fatalf("line %d lost its markers: %q", i, line)
		}
	}
}

func TestRenderSpannedMarksOnlyTheNamedBytes(t *testing.T) {
	st := plainStyles()
	const content = "hello world"
	keep, _ := spanIntervals(content, []eval.Span{span(0, 0, 5)}, 0)
	out := renderSpanned(content, keep, 80, st)
	if want := evidenceOpen + "hello" + evidenceClose + " world"; out != want {
		t.Fatalf("output = %q, want %q", out, want)
	}
}

// unicode.IsControl covers only category Cc, so the bidi overrides in Cf used to
// pass through the function documented as sanitising every untrusted string.
func TestPrintableRejectsBidiAndFormatRunes(t *testing.T) {
	for _, r := range []rune{'‮', '‭', '‏', '‎', '⁦', '⁩', '\x1b', '\x00', utf8.RuneError} {
		if printable(r) {
			t.Fatalf("printable(%U) = true, want false", r)
		}
	}
	for _, r := range []rune{'a', ' ', 'é', '漢', '😀'} {
		if !printable(r) {
			t.Fatalf("printable(%U) = false, want true", r)
		}
	}
	// A reversed verdict is the failure this protects against.
	out := renderSpanned("verdict: ‮dessap", nil, 80, plainStyles())
	if strings.ContainsRune(out, '‮') {
		t.Fatalf("bidi override survived: %q", out)
	}
}

// safeLabel guards the short identifiers that carry no spans: a newline in one of
// them used to make the runs table emit more lines than there were runs.
func TestSafeLabelIsOneLineOfPrintableText(t *testing.T) {
	got := safeLabel("sup\nport\ragent\tv2\x1b[2J‮")
	if strings.ContainsAny(got, "\n\r\t\x1b") || strings.ContainsRune(got, '‮') {
		t.Fatalf("safeLabel left something dangerous: %q", got)
	}
	if got != "sup port agent v2?[2J?" {
		t.Fatalf("safeLabel = %q", got)
	}
	if safeLabel("plain-id") != "plain-id" {
		t.Fatal("safeLabel altered a clean string")
	}
}

// A span that merely touched a line break used to render as "[[]]", and a span
// covering only a newline rendered as two empty pairs and nothing else.
func TestNoEmptyMarkersAroundNewlines(t *testing.T) {
	st := plainStyles()
	const content = "ab\ncd"
	for _, tc := range []struct{ lo, hi int }{
		{0, 3}, // ends at the newline
		{2, 5}, // starts at the newline
		{2, 3}, // the newline alone, which is what stepSpan can produce
		{0, 5}, // spans the break, and should be marked on both lines
	} {
		out := renderSpanned(content, mustIntervals(t, content, tc.lo, tc.hi), 40, st)
		if strings.Contains(out, evidenceOpen+evidenceClose) {
			t.Fatalf("[%d,%d) produced an empty marker pair: %q", tc.lo, tc.hi, out)
		}
	}
	// The multi-line span still marks both lines.
	out := renderSpanned(content, mustIntervals(t, content, 0, 5), 40, st)
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, evidenceOpen) {
			t.Fatalf("multi-line span lost a line's mark: %q", out)
		}
	}
}

func mustIntervals(t *testing.T, content string, lo, hi int) []interval {
	t.Helper()
	keep, bad := spanIntervals(content, []eval.Span{span(0, lo, hi)}, 0)
	if len(bad) != 0 {
		t.Fatalf("[%d,%d) refused: %#v", lo, hi, bad)
	}
	return keep
}

func TestClipStaysWithinWidth(t *testing.T) {
	if got := clip("abcdefghij", 5); lipgloss.Width(got) > 5 {
		t.Fatalf("clip = %q, %d cells", got, lipgloss.Width(got))
	}
	if got := clip("abc", 10); got != "abc" {
		t.Fatalf("clip shortened a fitting string: %q", got)
	}
	// Wide runes count as two cells.
	if got := clip("漢漢漢漢", 5); lipgloss.Width(got) > 5 {
		t.Fatalf("clip = %q, %d cells", got, lipgloss.Width(got))
	}
}

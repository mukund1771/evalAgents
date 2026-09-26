package tui

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"github.com/mukund1771/evalAgents/internal/eval"
)

// Evidence is marked in band, with ASCII, not only with colour.
//
// termenv returns styled text completely unmodified on its Ascii profile, and
// Ascii is what NO_COLOR, TERM=dumb and a non-terminal stdout all select. A
// highlight made only of escape sequences would therefore be invisible in
// exactly the situations where someone is most likely to be reading output they
// cannot see in colour, and invisible to a test asserting on View() as well.
// The markers are the signal; the style is an enhancement on top of them.
const (
	evidenceOpen  = "[["
	evidenceClose = "]]"
)

// printable reports whether a rune can be written to a terminal as itself.
//
// unicode.IsControl alone is not enough. It covers category Cc, which catches
// ESC and so stops a trajectory from clearing the screen, but it lets category
// Cf through - and that includes the bidi overrides. A final output of
// "verdict: \u202edessap" renders as "verdict: passed" in any terminal that
// honours them, which in a tool whose whole job is showing what the agent
// actually said is the worst possible failure.
func printable(r rune) bool {
	if r == utf8.RuneError { // also every invalid byte, so output is valid UTF-8
		return false
	}
	if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
		return false
	}
	return true
}

// safeLabel makes an untrusted string safe to place inside a rendered line.
//
// It is for short identifiers - a run id, a suite or case name, a step kind, a
// tool call id, a store path. Those carry no evidence spans, so unlike
// renderSpanned this may change the string's length freely. Every one of them
// reaches us from a file a user can edit or from the stdout of the program being
// evaluated, so none of them can be trusted to be one line of printable text.
//
// Collapsing newlines matters for more than looks: the runs table is built with
// a tabwriter and then read back line by line, so a name containing a newline
// would produce more lines than there are rows.
func safeLabel(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\r' || r == '\t':
			b.WriteByte(' ')
		case !printable(r):
			b.WriteByte('?')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// clip truncates to width display cells, marking the cut so a reader knows the
// line continues. Used for the lines outside the viewport, which bubbles does
// not clamp for us.
func clip(s string, width int) string {
	if width < 2 || runewidth.StringWidth(s) <= width {
		return s
	}
	return runewidth.Truncate(s, width, "\u2026")
}

// interval is a rune-aligned byte range inside one step's content, safe to slice.
type interval struct{ lo, hi int }

// spanIntervals turns the spans a scorer recorded into ranges that can be
// sliced out of content, and returns the ones it refuses separately.
//
// Offsets reach us from two untrusted places. They are read back from a JSON
// file a user can edit, and for the model-graded scorer they come straight from
// an LLM: parseJudge checks that StepIndex is in range and that End >= Start,
// but never that End is within the content or that Start is not negative.
//
// So a span is a claim, not a fact, and a bad one is reported rather than
// repaired. Clamping would paint a range the scorer never named, which is worse
// than painting nothing: the number would look audited when it is not. This is
// the same contract as a nil score value being inert instead of zero.
func spanIntervals(content string, spans []eval.Span, step int) (keep []interval, bad []eval.Span) {
	n := len(content)
	for _, sp := range spans {
		if sp.StepIndex != step {
			continue
		}
		if sp.Start < 0 || sp.End < sp.Start || sp.End > n {
			bad = append(bad, sp)
			continue
		}
		if sp.Start == sp.End {
			// Valid and empty. stepSpan emits this for a step with no content,
			// so it is not an anomaly and must not be reported as one.
			continue
		}
		// Widen to enclosing rune boundaries, never narrow: the highlight must
		// cover every byte the scorer named, and narrowing a one-rune span to
		// nothing would make the mark silently vanish.
		lo, hi := sp.Start, sp.End
		for lo > 0 && !utf8.RuneStart(content[lo]) {
			lo--
		}
		for hi < n && !utf8.RuneStart(content[hi]) {
			hi++
		}
		keep = append(keep, interval{lo, hi})
	}
	sort.Slice(keep, func(i, j int) bool {
		if keep[i].lo != keep[j].lo {
			return keep[i].lo < keep[j].lo
		}
		return keep[i].hi < keep[j].hi
	})
	return merge(keep), bad
}

// merge folds overlapping and touching intervals together. Several scorers point
// at the same final step, and without this the markers nest into [[a[[b]]c]].
func merge(in []interval) []interval {
	if len(in) < 2 {
		return in
	}
	out := in[:1]
	for _, iv := range in[1:] {
		last := &out[len(out)-1]
		if iv.lo <= last.hi {
			if iv.hi > last.hi {
				last.hi = iv.hi
			}
			continue
		}
		out = append(out, iv)
	}
	return out
}

// renderSpanned writes content with every interval marked, wrapped at width
// cells, and with control bytes neutralised.
//
// The three jobs are one pass on purpose. Wrapping separately would mean
// translating byte offsets across inserted newlines, and that arithmetic is
// where this kind of code goes wrong. Called with nil intervals it is the
// general-purpose sanitiser, so every untrusted string in the TUI goes through
// exactly one function.
//
// Neutralising control bytes is not cosmetic. Trajectory content is agent
// output, and this is the first thing to render it raw: a final answer
// containing an escape sequence would otherwise clear the user's screen.
func renderSpanned(content string, ivs []interval, width int, st styles) string {
	// Below this there is no room for a marked rune between the two markers.
	if width < 8 {
		width = 8
	}
	var out, run strings.Builder
	inside := false
	next := 0
	hi := 0
	col := 0

	// limit is the usable width of the current line. While a mark is open the
	// closing marker still has to fit, so those cells are reserved up front -
	// appending it afterwards is how a line ends up wider than the viewport.
	limit := func() int {
		if inside {
			return width - len(evidenceClose)
		}
		return width
	}
	flush := func() {
		if run.Len() == 0 {
			return
		}
		s := run.String()
		run.Reset()
		if inside {
			out.WriteString(st.evidence.Render(s))
			return
		}
		out.WriteString(s)
	}
	mark := func(s string) {
		flush()
		out.WriteString(st.evidence.Render(s))
		col += len(s) // the markers are ASCII, so bytes are cells
	}
	// newline closes and reopens the mark across the break, so neither a marker
	// nor a style sequence straddles a line: the viewport renders a slice of
	// lines, and an unterminated sequence leaks into whatever is drawn next.
	newline := func() {
		if inside {
			mark(evidenceClose)
		}
		flush()
		out.WriteByte('\n')
		col = 0
		if inside {
			mark(evidenceOpen)
		}
	}

	for i, r := range content {
		if inside && i >= hi {
			mark(evidenceClose)
			inside = false
		}
		// Skip an interval we have already walked past - one that covered only a
		// newline, for instance - so it cannot reopen as an empty marker pair.
		for next < len(ivs) && ivs[next].hi <= i {
			next++
		}
		// A newline is structure, not content. Opening or closing a mark across
		// one produced an empty "[[]]" for any span that merely touched a line
		// break, and for a span covering only a newline the evidence rendered as
		// two empty pairs and nothing else.
		if r == '\n' {
			cont := inside && hi > i+1
			if inside {
				mark(evidenceClose)
				inside = false
			}
			flush()
			out.WriteByte('\n')
			col = 0
			if cont {
				inside = true
				mark(evidenceOpen)
			}
			continue
		}
		if !inside && next < len(ivs) && i >= ivs[next].lo {
			// Opening needs room for both markers plus a rune between them.
			if col+len(evidenceOpen)+len(evidenceClose)+1 > width {
				newline()
			}
			mark(evidenceOpen)
			inside, hi = true, ivs[next].hi
			next++
		}

		switch {
		case r == '\t':
			// Four spaces, so the width is deterministic rather than
			// style-dependent. Three here and one from the common path below.
			for n := 0; n < 3; n++ {
				if col+1 > limit() {
					newline()
				}
				run.WriteByte(' ')
				col++
			}
			r = ' '
		case !printable(r):
			r = '?'
		}

		w := runewidth.RuneWidth(r)
		if w < 1 {
			w = 1
		}
		if col+w > limit() {
			newline()
		}
		run.WriteRune(r)
		col += w
	}
	if inside {
		mark(evidenceClose)
	}
	flush()
	return out.String()
}

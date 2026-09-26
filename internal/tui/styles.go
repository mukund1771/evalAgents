package tui

import "github.com/charmbracelet/lipgloss"

// styles holds every colour in the TUI, so the palette is one file to read.
//
// Only ANSI 0-15 is used, wrapped in AdaptiveColor for light and dark
// backgrounds. No truecolor hex: those have to be down-converted for terminals
// that cannot show them, and a palette that renders identically everywhere is
// worth more here than one extra shade.
//
// Every signal below also has a channel that survives losing colour entirely -
// the "> " cursor gutter, the "A"/"B" diff marks, the literal "-" for a skipped
// scorer, and the [[ ]] evidence markers - because lipgloss emits nothing at all
// on the Ascii profile.
type styles struct {
	title    lipgloss.Style
	header   lipgloss.Style
	cursor   lipgloss.Style
	mark     lipgloss.Style
	pass     lipgloss.Style
	fail     lipgloss.Style
	dim      lipgloss.Style
	evidence lipgloss.Style
	regress  lipgloss.Style
	note     lipgloss.Style
	errText  lipgloss.Style
}

func newStyles(r *lipgloss.Renderer) styles {
	var (
		red    = lipgloss.AdaptiveColor{Light: "1", Dark: "9"}
		green  = lipgloss.AdaptiveColor{Light: "2", Dark: "10"}
		yellow = lipgloss.AdaptiveColor{Light: "3", Dark: "11"}
		blue   = lipgloss.AdaptiveColor{Light: "4", Dark: "12"}
		grey   = lipgloss.AdaptiveColor{Light: "8", Dark: "8"}
	)
	return styles{
		title:    r.NewStyle().Bold(true),
		header:   r.NewStyle().Bold(true).Foreground(blue),
		cursor:   r.NewStyle().Bold(true),
		mark:     r.NewStyle().Bold(true).Foreground(blue),
		pass:     r.NewStyle().Foreground(green),
		fail:     r.NewStyle().Foreground(red),
		dim:      r.NewStyle().Foreground(grey),
		evidence: r.NewStyle().Reverse(true),
		regress:  r.NewStyle().Foreground(red),
		note:     r.NewStyle().Foreground(yellow),
		errText:  r.NewStyle().Bold(true).Foreground(red),
	}
}

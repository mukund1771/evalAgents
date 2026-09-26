package tui

import (
	"errors"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrNotATerminal is returned when the TUI is asked to draw somewhere it cannot.
var ErrNotATerminal = errors.New("tui needs a terminal; try agenteval list")

// Run opens the browser on the run store at root.
//
// This is the only exported function in the package and the only code here that
// touches a terminal, so everything else stays a pure state machine that tests
// can drive without a tty.
//
// out must be an *os.File: an alt-screen program driven by key input has no
// meaning against a pipe. Asserting the concrete type needs no new dependency
// and no syscall, and it gives the CLI test a deterministic way to prove the
// subcommand is wired without ever starting a program.
func Run(root string, out io.Writer) error {
	f, ok := out.(*os.File)
	if !ok {
		return ErrNotATerminal
	}
	p := tea.NewProgram(
		newModel(root, newStyles(lipgloss.DefaultRenderer())),
		tea.WithOutput(f),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}

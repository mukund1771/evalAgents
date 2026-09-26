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
// out must be a character device: an alt-screen program driven by key input has
// no meaning against a pipe or a file.
func Run(root string, out io.Writer) error {
	f, ok := out.(*os.File)
	if !ok {
		return ErrNotATerminal
	}
	// *os.File alone was not enough: a redirect satisfies it, so
	// `agenteval tui > runs.txt` entered the alt screen against a file and hung
	// with nothing on screen instead of printing the advice above. The mode bit
	// is the stdlib way to ask, so this still needs no new dependency, and a
	// bytes.Buffer still fails, which is how the CLI test proves the subcommand
	// is wired without starting a program.
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return ErrNotATerminal
	}
	p := tea.NewProgram(
		newModel(root, newStyles(lipgloss.DefaultRenderer())),
		tea.WithOutput(f),
		tea.WithAltScreen(),
	)
	_, err = p.Run()
	return err
}

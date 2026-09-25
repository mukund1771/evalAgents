package report

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"agenteval/internal/store"
)

// Table writes a pass^k summary. A dash means the scorer skipped.
func Table(w io.Writer, sums ...store.Summary) {
	for i, sum := range sums {
		if i > 0 {
			fmt.Fprintln(w)
		}
		fmt.Fprintf(w, "run %s  suite %s  pass^1 %.2f  pass^k %.2f\n", sum.RunID, sum.Suite, sum.Pass1, sum.PassK)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		headers := []string{"CASE", "PASS^1", "PASS^K"}
		for _, sc := range sum.Scorers {
			headers = append(headers, sc.Name)
		}
		fmt.Fprintln(tw, strings.Join(headers, "\t"))
		for _, c := range sum.Cases {
			cols := []string{c.ID, fmt.Sprintf("%.2f", c.Pass1), fmt.Sprintf("%.2f", c.PassK)}
			for _, sc := range sum.Scorers {
				if v, ok := c.Means[sc.Name]; ok {
					cols = append(cols, fmt.Sprintf("%.2f", v))
				} else {
					cols = append(cols, "-")
				}
			}
			fmt.Fprintln(tw, strings.Join(cols, "\t"))
		}
		tw.Flush()
	}
}

// MatrixTable compares one run per matrix value.
func MatrixTable(w io.Writer, sums []store.Summary) {
	if len(sums) < 2 {
		return
	}
	fmt.Fprintln(w, "\nmatrix")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "VALUE\tRUN\tPASS^1\tPASS^K")
	for _, s := range sums {
		fmt.Fprintf(tw, "%s\t%s\t%.2f\t%.2f\n", s.Matrix, s.RunID, s.Pass1, s.PassK)
	}
	tw.Flush()
}

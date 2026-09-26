package report

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"

	"github.com/mukund1771/evalAgents/internal/store"
)

type testSuites struct {
	XMLName xml.Name   `xml:"testsuites"`
	Suites  []xmlSuite `xml:"testsuite"`
}

type xmlSuite struct {
	Name     string    `xml:"name,attr"`
	Tests    int       `xml:"tests,attr"`
	Failures int       `xml:"failures,attr"`
	Skipped  int       `xml:"skipped,attr"`
	Cases    []xmlCase `xml:"testcase"`
}

type xmlCase struct {
	Name    string  `xml:"name,attr"`
	Class   string  `xml:"classname,attr"`
	Failure *xmlMsg `xml:"failure,omitempty"`
	Skipped *xmlMsg `xml:"skipped,omitempty"`
}

type xmlMsg struct {
	Message string `xml:"message,attr"`
	Body    string `xml:",chardata"`
}

// WriteJUnit writes one testcase per eval case.
func WriteJUnit(path string, sum store.Summary) error {
	xs := xmlSuite{Name: sum.Suite}
	for _, c := range sum.Cases {
		xs.Tests++
		tc := xmlCase{Name: c.ID, Class: sum.Suite}
		if !c.Passed {
			xs.Failures++
			msg := "case failed"
			if len(c.Failed) > 0 {
				msg = "failed: " + strings.Join(c.Failed, ", ")
			}
			tc.Failure = &xmlMsg{Message: msg, Body: msg}
		}
		xs.Cases = append(xs.Cases, tc)
	}
	doc := testSuites{Suites: []xmlSuite{xs}}
	b, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	out := append([]byte(xml.Header), b...)
	out = append(out, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

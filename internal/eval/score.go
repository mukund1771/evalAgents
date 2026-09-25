package eval

// Float returns a score with Passed set from value >= min.
func Float(name string, value, min float64, explanation string, evidence []Span) Score {
	passed := value >= min
	return Score{
		Name:        name,
		Value:       &value,
		Passed:      &passed,
		Explanation: explanation,
		Evidence:    evidence,
	}
}

// Skip returns a score whose nil value means the scorer did not apply.
func Skip(name, explanation string) Score {
	return Score{Name: name, Value: nil, Explanation: explanation}
}

// Applicable reports whether the score was computed.
func (s Score) Applicable() bool {
	return s.Value != nil
}

// OK reports whether an applicable score passed. A skip is not a pass.
func (s Score) OK() bool {
	return s.Passed != nil && *s.Passed
}

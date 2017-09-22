package annotations

import (
	"context"
	"regexp"
)

type IDLinter struct {
	pattern     *regexp.Regexp
	replacement string
}

func NewIDLinter(from string, replacement string) (*IDLinter, error) {
	var rule *IDLinter
	pattern, err := regexp.Compile(from)
	if err == nil {
		rule = &IDLinter{pattern, replacement}
	}

	return rule, err
}

func (l *IDLinter) Lint(conceptId string) string {
	return l.pattern.ReplaceAllString(conceptId, l.replacement)
}

func (l *IDLinter) Apply(ctx context.Context, in []Annotation) ([]Annotation, error) {
	out := []Annotation{}

	for _, ann := range in {
		out = append(out, Annotation{
			Predicate: ann.Predicate,
			ConceptId: l.Lint(ann.ConceptId),
			ApiUrl:    ann.ApiUrl,
			Types:     ann.Types,
			PrefLabel: ann.PrefLabel,
		})
	}

	return out, nil
}

func (l *IDLinter) String() string {
	return "ID linter"
}

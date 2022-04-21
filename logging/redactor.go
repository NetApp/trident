package logging

import (
	"regexp"

	"github.com/sirupsen/logrus"
)

type redactedPattern struct {
	re  *regexp.Regexp
	rep []byte
}

// Redacted patterns and their replacements - add additional patterns here
var redactedPatterns = []redactedPattern{
	basicAuthorization,
}

// Redactor is a formatter that redacts pre-defined regex patterns
type Redactor struct {
	BaseFormatter logrus.Formatter
}

func (r *Redactor) Format(entry *logrus.Entry) ([]byte, error) {
	line, err := r.BaseFormatter.Format(entry)
	return redactAllPatterns(line), err
}

func redactAllPatterns(line []byte) []byte {
	for _, rp := range redactedPatterns {
		line = rp.re.ReplaceAll(line, rp.rep)
	}

	return line
}

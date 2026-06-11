package logging

import (
	"regexp"

	log "github.com/sirupsen/logrus"
)

type redactedPattern struct {
	re  *regexp.Regexp
	rep []byte
}

// Redacted patterns and their replacements - add additional patterns here
var redactedPatterns = []redactedPattern{
	basicAuthorization,
	csiSecrets,
	chapAuthorization,
	chapAuthorizationUser,
	backendCreateCHAPSecrets,
	backendCreateCHAPUsername,
	backendAuthorization,
	backendSensitiveMap,
}

// Redactor is a formatter that redacts pre-defined regex patterns
type Redactor struct {
	BaseFormatter log.Formatter
}

func (r *Redactor) Format(entry *log.Entry) ([]byte, error) {
	line, err := r.BaseFormatter.Format(entry)
	return redactAllPatterns(line), err
}

// RedactBytes applies all configured logging redaction patterns to raw bytes.
func RedactBytes(line []byte) []byte {
	return redactAllPatterns(line)
}

// RedactString applies all configured logging redaction patterns to a string.
func RedactString(line string) string {
	return string(redactAllPatterns([]byte(line)))
}

func redactAllPatterns(line []byte) []byte {
	for _, rp := range redactedPatterns {
		line = rp.re.ReplaceAll(line, rp.rep)
	}

	return line
}

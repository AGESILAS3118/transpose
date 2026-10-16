package repos

import (
	"net/url"
	"regexp"
	"strings"
)

var sensitiveLinePattern = regexp.MustCompile(`(?i)(password|token|apikey|secret|private_key|private key|BEGIN PGP PRIVATE KEY|BEGIN (OPENSSH |RSA |EC )?PRIVATE KEY)`)

func sanitizeContent(content string) string {
	lines := strings.Split(content, "\n")
	output := make([]string, 0, len(lines))
	skipPrivateBlock := false
	for _, line := range lines {
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "BEGIN PGP PRIVATE KEY") ||
			strings.Contains(upper, "BEGIN OPENSSH PRIVATE KEY") ||
			strings.Contains(upper, "BEGIN RSA PRIVATE KEY") ||
			strings.Contains(upper, "BEGIN EC PRIVATE KEY") ||
			strings.Contains(upper, "BEGIN PRIVATE KEY") {
			skipPrivateBlock = true
			continue
		}
		if skipPrivateBlock {
			if strings.Contains(upper, "END PGP PRIVATE KEY") ||
				strings.Contains(upper, "END OPENSSH PRIVATE KEY") ||
				strings.Contains(upper, "END RSA PRIVATE KEY") ||
				strings.Contains(upper, "END EC PRIVATE KEY") ||
				strings.Contains(upper, "END PRIVATE KEY") {
				skipPrivateBlock = false
			}
			continue
		}
		if sensitiveLinePattern.MatchString(line) {
			continue
		}
		output = append(output, redactURLCredentials(line))
	}
	return strings.Join(output, "\n")
}

func redactURLCredentials(line string) string {
	fields := strings.Fields(line)
	for index, field := range fields {
		if strings.Contains(field, "://") {
			fields[index] = sanitizeReference(field)
		}
	}
	return strings.Join(fields, " ")
}

func sanitizeReference(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	parsed.User = nil
	query := parsed.Query()
	for key := range query {
		if sensitiveLinePattern.MatchString(key) {
			query.Del(key)
		}
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

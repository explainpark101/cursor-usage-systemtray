package auth

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseCookieInput accepts either a Netscape cookies.txt body (curl cookie
// jar) or a raw Cookie request-header value, and returns a Cookie header
// string filtered to cursor.com cookies when the Netscape format is used.
//
// Netscape format (https://curl.se/docs/http-cookies.html), one cookie per
// line, TAB-separated fields:
//
//	domain  includeSubdomains  path  secure  expiration  name  value
//
// Comment lines start with '#', except the '#HttpOnly_' domain prefix.
func ParseCookieInput(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("cookie input is empty")
	}
	if looksLikeNetscapeCookieFile(raw) {
		header, err := parseNetscapeCookieFile(raw)
		if err != nil {
			return "", err
		}
		if header == "" {
			return "", fmt.Errorf("no cursor.com cookies found in cookie file")
		}
		return header, nil
	}
	header := trimCookieHeader(raw)
	if header == "" {
		return "", fmt.Errorf("cookie value is empty")
	}
	header = normalizeCookieHeader(header)
	return header, nil
}

// normalizeCookieHeader strips a leading "Cookie:" label and, when the user
// pasted only the WorkosCursorSessionToken value, wraps it as a proper pair.
func normalizeCookieHeader(header string) string {
	lower := strings.ToLower(header)
	if strings.HasPrefix(lower, "cookie:") {
		header = strings.TrimSpace(header[len("cookie:"):])
	}
	// Bare token paste (no '=' and no ';') → assume WorkosCursorSessionToken.
	if !strings.Contains(header, "=") && !strings.Contains(header, ";") && header != "" {
		return "WorkosCursorSessionToken=" + header
	}
	return header
}

func looksLikeNetscapeCookieFile(raw string) bool {
	lower := strings.ToLower(raw)
	if strings.Contains(lower, "netscape http cookie file") ||
		strings.Contains(lower, "# httponly_") ||
		strings.Contains(raw, "#HttpOnly_") {
		return true
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.Count(line, "\t") >= 6 {
			return true
		}
		// Some exporters use spaces; still require many columns.
		fields := strings.Fields(line)
		if len(fields) >= 7 && (fields[1] == "TRUE" || fields[1] == "FALSE") {
			return true
		}
	}
	return false
}

func parseNetscapeCookieFile(raw string) (string, error) {
	now := time.Now().Unix()
	var parts []string
	seen := map[string]struct{}{}
	lines := strings.Split(raw, "\n")
	parsed := 0

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		httpOnly := false
		if strings.HasPrefix(line, "#HttpOnly_") {
			httpOnly = true
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		_ = httpOnly // attribute is informational for Cookie header export

		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			// Fall back to whitespace split for slightly non-conforming files.
			fields = strings.Fields(line)
			if len(fields) < 7 {
				continue
			}
			// Value may contain spaces: join remaining fields.
			if len(fields) > 7 {
				fields = append(fields[:6], strings.Join(fields[6:], " "))
			}
		} else if len(fields) > 7 {
			// Value field may itself contain tabs; join the tail.
			fields = append(fields[:6], strings.Join(fields[6:], "\t"))
		}

		domain := strings.TrimSpace(fields[0])
		name := strings.TrimSpace(fields[5])
		value := fields[6]
		if domain == "" || name == "" {
			continue
		}
		if !isCursorDomain(domain) {
			continue
		}

		expStr := strings.TrimSpace(fields[4])
		if expStr != "" && expStr != "0" {
			exp, err := strconv.ParseInt(expStr, 10, 64)
			if err == nil && exp > 0 && exp < now {
				continue // expired
			}
		}

		key := name
		if _, ok := seen[key]; ok {
			// Later lines win (file order), matching common jar behavior.
			for i, p := range parts {
				if strings.HasPrefix(p, name+"=") {
					parts = append(parts[:i], parts[i+1:]...)
					break
				}
			}
		}
		seen[key] = struct{}{}
		parts = append(parts, name+"="+value)
		parsed++
	}

	if parsed == 0 && len(parts) == 0 {
		// Distinguish "bad file" vs "no matching domain".
		if hasAnyCookieLine(raw) {
			return "", nil
		}
		return "", fmt.Errorf("no cookie lines found (expected Netscape cookies.txt)")
	}
	return strings.Join(parts, "; "), nil
}

func hasAnyCookieLine(raw string) bool {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, "#HttpOnly_") {
			return true
		}
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if strings.Count(line, "\t") >= 6 || len(strings.Fields(line)) >= 7 {
			return true
		}
	}
	return false
}

func isCursorDomain(domain string) bool {
	d := strings.ToLower(strings.TrimPrefix(domain, "."))
	return d == "cursor.com" || strings.HasSuffix(d, ".cursor.com")
}

func trimCookieHeader(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		out = append(out, r)
	}
	res := make([]rune, 0, len(out))
	prevSpace := false
	for _, r := range out {
		if r == ' ' {
			if prevSpace {
				continue
			}
			prevSpace = true
		} else {
			prevSpace = false
		}
		res = append(res, r)
	}
	s = string(res)
	return strings.TrimSpace(s)
}

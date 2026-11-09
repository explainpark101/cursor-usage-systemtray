package auth

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestParseNetscapeCookieFile(t *testing.T) {
	raw := `# Netscape HTTP Cookie File
# https://curl.se/docs/http-cookies.html
.cursor.com	TRUE	/	TRUE	0	session	abc123
#HttpOnly_.cursor.com	TRUE	/	TRUE	0	WorkosCursorSessionToken	tok%3Dval
.example.com	TRUE	/	FALSE	0	other	nope
`
	got, err := ParseCookieInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "session=abc123") {
		t.Fatalf("missing session: %q", got)
	}
	if !strings.Contains(got, "WorkosCursorSessionToken=tok%3Dval") {
		t.Fatalf("missing httponly token: %q", got)
	}
	if strings.Contains(got, "other=") {
		t.Fatalf("leaked foreign domain: %q", got)
	}
}

func TestParseNetscapeSkipsExpired(t *testing.T) {
	past := time.Now().Add(-time.Hour).Unix()
	raw := ".cursor.com\tTRUE\t/\tTRUE\t" + strconv.FormatInt(past, 10) + "\told\tgone\n" +
		".cursor.com\tTRUE\t/\tTRUE\t0\tfresh\tok\n"
	got, err := ParseCookieInput(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fresh=ok" {
		t.Fatalf("got %q", got)
	}
}

func TestParseCookieHeaderPassthrough(t *testing.T) {
	got, err := ParseCookieInput("  a=1; b=2 \n")
	if err != nil {
		t.Fatal(err)
	}
	if got != "a=1; b=2" {
		t.Fatalf("got %q", got)
	}
}

func TestParseCookieHeaderPrefixAndBareToken(t *testing.T) {
	got, err := ParseCookieInput("Cookie: WorkosCursorSessionToken=abc%3A%3Axyz")
	if err != nil {
		t.Fatal(err)
	}
	if got != "WorkosCursorSessionToken=abc%3A%3Axyz" {
		t.Fatalf("got %q", got)
	}
	got, err = ParseCookieInput("user_01%3A%3AeyJhbGciOi")
	if err != nil {
		t.Fatal(err)
	}
	if got != "WorkosCursorSessionToken=user_01%3A%3AeyJhbGciOi" {
		t.Fatalf("bare token wrap: %q", got)
	}
}

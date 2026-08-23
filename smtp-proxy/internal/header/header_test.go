package header

import (
	"bytes"
	"testing"
)

func TestInject(t *testing.T) {
	raw := []byte("Subject: hi\r\nFrom: a@x.com\r\n\r\nbody text")

	got := Inject(raw, "HIGH", []string{"credentials", "internal-only"}, "redirect")

	want := "X-ComplyMail-Sensitivity: HIGH\r\n" +
		"X-ComplyMail-Flags: credentials, internal-only\r\n" +
		"X-ComplyMail-Action: redirect\r\n" +
		"Subject: hi\r\nFrom: a@x.com\r\n\r\nbody text"

	if string(got) != want {
		t.Fatalf("Inject() =\n%q\nwant\n%q", got, want)
	}

	if !bytes.HasSuffix(got, raw) {
		t.Error("Inject() must leave the original raw message untouched, appended verbatim")
	}
}

func TestInject_EmptyFlags(t *testing.T) {
	raw := []byte("Subject: hi\r\n\r\nbody")

	got := Inject(raw, "LOW", nil, "pass")

	if !bytes.Contains(got, []byte("X-ComplyMail-Flags: \r\n")) {
		t.Errorf("expected empty Flags header, got %q", got)
	}
}

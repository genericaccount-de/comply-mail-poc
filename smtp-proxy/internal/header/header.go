// Package header injects X-ComplyMail-* headers into raw outbound email
// messages so downstream mail clients/reviewers can see why a message was
// flagged or redirected.
package header

import (
	"bytes"
	"fmt"
	"strings"
)

// Header names stamped onto flagged/redirected messages.
const (
	Sensitivity = "X-ComplyMail-Sensitivity"
	Flags       = "X-ComplyMail-Flags"
	Action      = "X-ComplyMail-Action"
)

// Inject prepends ComplyMail headers to a raw RFC 5322 message. It does not
// parse or otherwise touch existing headers or the body: RFC 5322 permits
// header fields in any order before the blank line separating headers from
// body, so prepending is sufficient and avoids locating that boundary.
func Inject(raw []byte, sensitivity string, flags []string, action string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s: %s\r\n", Sensitivity, sensitivity)
	fmt.Fprintf(&buf, "%s: %s\r\n", Flags, strings.Join(flags, ", "))
	fmt.Fprintf(&buf, "%s: %s\r\n", Action, action)
	buf.Write(raw)
	return buf.Bytes()
}

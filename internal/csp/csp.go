// Package csp computes the Content-Security-Policy hash sources of the
// inline elements go-partial writes.
package csp

import (
	"crypto/sha256"
	"encoding/base64"
)

// Hash returns the hash source of an inline <style> or <script> element
// whose text is text, everything between its start and end tag, exactly
// as it is written: 'sha256-…', with its quotes, ready for a policy's
// style-src or script-src directive.
func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))

	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}

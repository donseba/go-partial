package csp

import "testing"

func TestHashOfKnownText(t *testing.T) {
	// The hash of the empty text, as browsers report it for an empty
	// <style></style>.
	if got, want := Hash(""), "'sha256-47DEQpj8HBSa+/TImW+5JCeuQeRkm5NMpJWZG3hSuFU='"; got != want {
		t.Fatalf("Hash(\"\") = %s, want %s", got, want)
	}
}

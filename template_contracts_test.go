package partial

import "testing"

func TestParseTemplateContractModels(t *testing.T) {
	contract := parseTemplateContract(`{{/*
@partial users/row
@model User User
@model Can Permissions
*/}}
<p>{{ User.Name }}</p>`)

	user, ok := contract.Models["User"]
	if !ok {
		t.Fatal("expected User model")
	}
	if user.Type != "User" {
		t.Fatalf("user type = %q, want User", user.Type)
	}

	can, ok := contract.Models["Can"]
	if !ok {
		t.Fatal("expected Can model")
	}
	if can.Type != "Permissions" {
		t.Fatalf("can type = %q, want Permissions", can.Type)
	}
}

package api

import "testing"

func TestValidJSONPCallbackAcceptsDottedIdentifiers(t *testing.T) {
	if !validJSONPCallback("Stint.widget_1.render") {
		t.Fatal("expected dotted callback identifier to be accepted")
	}
}

func TestValidJSONPCallbackRejectsExecutableInput(t *testing.T) {
	for _, value := range []string{"alert(1)", "x-y", "window['x']", "a/b"} {
		if validJSONPCallback(value) {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
}

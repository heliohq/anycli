package tools

import "testing"

func TestXServiceIsRegistered(t *testing.T) {
	if !HasService("x") {
		t.Fatal("x built-in service is not registered")
	}
}

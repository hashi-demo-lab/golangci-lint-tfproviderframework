package testlintdata

import (
	"testing"
)

// This test file exists but has NO TestAcc function
// This should trigger a diagnostic: resource with test file but no TestAcc function

func TestWidget_NotAnAcceptanceTest(t *testing.T) {
	// This is just a regular unit test, not an acceptance test
	t.Log("This is not an acceptance test")
}

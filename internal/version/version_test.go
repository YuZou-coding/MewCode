package version

import "testing"

func TestStringUsesDevelopmentVersionByDefault(t *testing.T) {
	if got := String(); got != "MewCode dev" {
		t.Fatalf("String() = %q", got)
	}
}

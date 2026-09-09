package acceptance

import (
	"regexp"
	"testing"
)

func TestUniqueNameFormat(t *testing.T) {
	name := UniqueName("live")
	re := regexp.MustCompile(`^acc_live_[0-9a-f]{8}$`)
	if !re.MatchString(name) {
		t.Fatalf("UniqueName(\"live\") = %q, want acc_live_<digits>", name)
	}
}

func TestUniqueNameDistinct(t *testing.T) {
	if UniqueName("live") == UniqueName("live") {
		t.Fatal("UniqueName returned the same value twice")
	}
}

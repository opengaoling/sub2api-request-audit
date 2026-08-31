package service

import "testing"

func TestCRSSchedulable(t *testing.T) {
	explicitFalse := false
	explicitTrue := true

	if got := crsSchedulable(nil, true); !got {
		t.Fatal("missing schedulable should use the provided default")
	}
	if got := crsSchedulable(&explicitFalse, true); got {
		t.Fatal("explicit false schedulable should be preserved")
	}
	if got := crsSchedulable(&explicitTrue, false); !got {
		t.Fatal("explicit true schedulable should be preserved")
	}
}

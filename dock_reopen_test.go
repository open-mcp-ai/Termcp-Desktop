package main

import "testing"

func TestWindowReopenControllerDispatchesCurrentHandler(t *testing.T) {
	var controller windowReopenController
	calls := 0

	controller.reopen()
	controller.set(func() { calls++ })
	controller.reopen()
	controller.set(nil)
	controller.reopen()

	if calls != 1 {
		t.Fatalf("reopen handler calls = %d, want 1", calls)
	}
}

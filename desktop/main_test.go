package main

import "testing"

func TestMainWindowOptionsUseCompactResizableDefaults(t *testing.T) {
	options := mainWindowOptions()

	if options.Width != 680 || options.Height != 720 {
		t.Fatalf("default window size = %dx%d, want 680x720", options.Width, options.Height)
	}
	if options.MinWidth != 560 || options.MinHeight != 560 {
		t.Fatalf("minimum window size = %dx%d, want 560x560", options.MinWidth, options.MinHeight)
	}
	if options.Width < options.MinWidth || options.Height < options.MinHeight {
		t.Fatalf("default window size must not be below its minimum: %+v", options)
	}
}

package backend

import "testing"

func TestRealMacIcons(t *testing.T) {
	realIconTest(t, "/System/Applications/Calculator.app/Contents/MacOS/Calculator", "Calculator")
	realIconTest(t, "/System/Library/CoreServices/Finder.app/Contents/MacOS/Finder", "Finder")
}

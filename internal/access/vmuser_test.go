package access

import (
	"regexp"
	"testing"
)

func TestVMUsername(t *testing.T) {
	valid := regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,19}$`)
	for in, want := range map[string]string{
		"bioku4real@gmail.com":                     "bioku4real",
		"Rita.Smith@example.org":                   "rita-smith",
		"o'brien+tre@example.org":                  "o-brien-tre",
		"1stuser@example.org":                      "u1stuser",
		"a.very.long.name.indeed.here@example.org": "a-very-long-name-ind",
		"admin@example.org":                        FallbackVMUsername,
		"Administrator@example.org":                FallbackVMUsername,
		"...@example.org":                          FallbackVMUsername,
		"":                                         FallbackVMUsername,
		"0c0e-subject-without-email":               "u0c0e-subject-withou",
	} {
		got := VMUsername(in)
		if got != want {
			t.Errorf("VMUsername(%q) = %q, want %q", in, got, want)
		}
		if !valid.MatchString(got) || reservedVMUsernames[got] {
			t.Errorf("VMUsername(%q) = %q is not a valid Azure admin username", in, got)
		}
	}
}

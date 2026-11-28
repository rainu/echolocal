package openclaw

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSessionTitleMetadataPrecedenceAndSanitization(t *testing.T) {
	for _, c := range []struct {
		in   []string
		want string
	}{
		{[]string{"Renamed", "Old display", "Derived"}, "Renamed"},
		{[]string{"", "Display", "Derived"}, "Display"},
		{[]string{"", "", "Derived"}, "Derived"},
		{[]string{"agent:test:one", "", ""}, ""},
		{[]string{"agent:private:key", "Safe title"}, "Safe title"},
		{[]string{"  Unicode\n日本語\t ä\x00 title  "}, "Unicode 日本語 ä title"},
		{nil, ""},
	} {
		if got := sessionTitle("agent:test:one", c.in...); got != c.want {
			t.Fatalf("%q != %q", got, c.want)
		}
	}
	if got := sessionTitle("agent:test:one", strings.Repeat("界", 900)); utf8.RuneCountInString(got) != 512 || !strings.HasSuffix(got, "…") {
		t.Fatal("unbounded title")
	}
}

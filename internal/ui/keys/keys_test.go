package keys

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// Scope stacks the app uses, most specific first.
var stacks = [][]Scope{
	{Sidebar, Nav},
	{Diff, Nav},
	{Visual, Diff, Nav},
	{Chat, Nav},
}

func feedAll(r *Resolver, seq string, scopes ...Scope) Result {
	var res Result
	for _, k := range strings.Fields(seq) {
		res = r.Feed(k, scopes...)
	}
	return res
}

func TestEveryBindingIsReachable(t *testing.T) {
	for _, stack := range stacks {
		for _, b := range Bindings {
			if !inStack(b.Scope, stack) {
				continue
			}
			var r Resolver
			res := feedAll(&r, b.Keys, stack...)
			if res.Action == "" {
				if !shadowedByMoreSpecific(b, stack) {
					t.Errorf("%v: %q (%v) is unreachable: %+v", stack, b.Keys, b.Action, res)
				}
				continue
			}
			if res.Action != b.Action && !shadowedByMoreSpecific(b, stack) {
				t.Errorf("%v: %q gives %v, want %v", stack, b.Keys, res.Action, b.Action)
			}
			if r.Busy() {
				t.Errorf("%v: %q left the resolver busy", stack, b.Keys)
			}
		}
	}
}

func TestNoAmbiguityWithinAScope(t *testing.T) {
	for _, a := range Bindings {
		for _, b := range Bindings {
			if a.Scope == b.Scope && strings.HasPrefix(b.Keys, a.Keys+" ") {
				t.Errorf("%v: %q is a prefix of %q", a.Scope, a.Keys, b.Keys)
			}
		}
	}
}

func TestAliasesShareHelp(t *testing.T) {
	help := map[[2]string]string{}
	for _, b := range Bindings {
		k := [2]string{b.Scope.String(), string(b.Action)}
		if h, ok := help[k]; ok && h != b.Help {
			t.Errorf("%v %v: help %q vs %q", b.Scope, b.Action, h, b.Help)
		}
		help[k] = b.Help
	}
}

func TestCounts(t *testing.T) {
	var r Resolver
	if res := feedAll(&r, "1 2 j", Diff, Nav); res.Action != Down || res.Count != 12 {
		t.Fatalf("12j = %+v", res)
	}
	if res := feedAll(&r, "0", Diff, Nav); res.Action != ScrollHome || res.Count != 0 {
		t.Fatalf("0 = %+v", res)
	}
	if res := feedAll(&r, "1 0 G", Diff, Nav); res.Action != Bottom || res.Count != 10 {
		t.Fatalf("10G = %+v", res)
	}
	if res := feedAll(&r, "3 ] c", Diff, Nav); res.Action != NextChange || res.Count != 3 {
		t.Fatalf("3]c = %+v", res)
	}
}

func TestPendingAndUnbound(t *testing.T) {
	var r Resolver
	if res := r.Feed("g", Diff, Nav); !res.Pending {
		t.Fatalf("g should be pending: %+v", res)
	}
	if got := r.Pending(); got != "g" {
		t.Fatalf("Pending() = %q", got)
	}
	seq := r.Seq()
	if res := r.Feed("x", Diff, Nav); !res.Unbound || r.Busy() {
		t.Fatalf("gx should be unbound and reset: %+v", res)
	}
	if r.Seq() == seq {
		t.Fatal("Seq must change when pending state is dropped")
	}
	if res := feedAll(&r, "space a", Diff, Nav); res.Action != ToggleChat {
		t.Fatalf("<space>a = %+v", res)
	}
}

func TestScopePrecedence(t *testing.T) {
	var r Resolver
	if res := r.Feed("q", Chat, Nav); res.Action != ChatHide {
		t.Fatalf("q in chat = %+v", res)
	}
	if res := r.Feed("q", Diff, Nav); res.Action != Quit {
		t.Fatalf("q in diff = %+v", res)
	}
	if res := r.Feed("y", Visual, Diff, Nav); res.Action != Yank {
		t.Fatalf("y in visual = %+v", res)
	}
	if res := feedAll(&r, "y y", Diff, Nav); res.Action != YankLine {
		t.Fatalf("yy in diff = %+v", res)
	}
	if res := feedAll(&r, "] f", Diff, Nav); res.Action != NextFile {
		t.Fatalf("]f in diff = %+v", res)
	}
	if res := r.Feed("l", Sidebar, Nav); res.Action != Expand {
		t.Fatalf("l in sidebar = %+v", res)
	}
	if res := r.Feed("l", Diff, Nav); res.Action != ScrollRight {
		t.Fatalf("l in diff = %+v", res)
	}
}

func TestNameMapsCyrillicLayout(t *testing.T) {
	cases := []struct {
		key  tea.KeyPressMsg
		want string
	}{
		{tea.KeyPressMsg{Code: 'j', Text: "j"}, "j"},
		{tea.KeyPressMsg{Code: 'о', Text: "о"}, "j"},
		{tea.KeyPressMsg{Code: 'О', Text: "О", Mod: tea.ModShift}, "J"},
		{tea.KeyPressMsg{Code: 'п', Text: "п", BaseCode: 'g'}, "g"},
		{tea.KeyPressMsg{Code: 'Р', Text: "Р", BaseCode: 'h', Mod: tea.ModShift}, "H"},
		{tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, "ctrl+d"},
		{tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl | tea.ModShift}, "ctrl+shift+d"},
		{tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, "space"},
		{tea.KeyPressMsg{Code: '?', Text: "?"}, "?"},
		{tea.KeyPressMsg{Code: 'ё', Text: "ё"}, "ё"},
	}
	for _, c := range cases {
		if got := Name(c.key); got != c.want {
			t.Errorf("Name(%q) = %q, want %q", c.key.Text, got, c.want)
		}
	}
}

func inStack(s Scope, stack []Scope) bool {
	if s == Global {
		return true
	}
	for _, x := range stack {
		if x == s {
			return true
		}
	}
	return false
}

// shadowedByMoreSpecific reports whether an earlier scope in the stack binds
// the same keys or a prefix of them, which is intended (e.g. "q" hides the
// Claude panel, "y" in visual mode yanks before "yy" could complete).
func shadowedByMoreSpecific(b Binding, stack []Scope) bool {
	for _, s := range withGlobal(stack) {
		if s == b.Scope {
			return false
		}
		for _, o := range Bindings {
			if o.Scope == s && (o.Keys == b.Keys || strings.HasPrefix(b.Keys, o.Keys+" ")) {
				return true
			}
		}
	}
	return false
}

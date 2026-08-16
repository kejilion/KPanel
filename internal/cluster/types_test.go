package cluster

import "testing"

func TestBuildV2ScopeIsCanonicalAndValid(t *testing.T) {
	cases := []struct {
		name        string
		terminal    bool
		browseFetch bool
		browseWS    bool
		want        string
	}{
		{"summary only", false, false, false, "cluster.summary.read"},
		{"summary + terminal", true, false, false, "cluster.summary.read cluster.terminal.open"},
		{"summary + browse fetch", false, true, false, "cluster.summary.read cluster.browse.fetch"},
		{"summary + browse ws", false, false, true, "cluster.summary.read cluster.browse.ws"},
		{"summary + terminal + browse fetch", true, true, false, "cluster.summary.read cluster.terminal.open cluster.browse.fetch"},
		{"summary + terminal + browse ws", true, false, true, "cluster.summary.read cluster.terminal.open cluster.browse.ws"},
		{"summary + browse fetch + browse ws", false, true, true, "cluster.summary.read cluster.browse.fetch cluster.browse.ws"},
		{"all four", true, true, true, "cluster.summary.read cluster.terminal.open cluster.browse.fetch cluster.browse.ws"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := BuildV2Scope(testCase.terminal, testCase.browseFetch, testCase.browseWS)
			if got != testCase.want {
				t.Fatalf("BuildV2Scope(%v, %v, %v) = %q, want %q", testCase.terminal, testCase.browseFetch, testCase.browseWS, got, testCase.want)
			}
			if !ValidV2Scope(got) {
				t.Fatalf("ValidV2Scope(%q) = false, want true", got)
			}
			if ScopeAllowsTerminal(got) != testCase.terminal {
				t.Fatalf("ScopeAllowsTerminal(%q) = %v, want %v", got, ScopeAllowsTerminal(got), testCase.terminal)
			}
			if ScopeAllowsBrowse(got) != testCase.browseFetch {
				t.Fatalf("ScopeAllowsBrowse(%q) = %v, want %v", got, ScopeAllowsBrowse(got), testCase.browseFetch)
			}
			if ScopeAllowsBrowseWS(got) != testCase.browseWS {
				t.Fatalf("ScopeAllowsBrowseWS(%q) = %v, want %v", got, ScopeAllowsBrowseWS(got), testCase.browseWS)
			}
		})
	}
}

func TestValidV2ScopeRejectsMalformedInput(t *testing.T) {
	cases := []string{
		"",
		"cluster.terminal.open", // missing mandatory summary token
		"cluster.browse.fetch cluster.summary.read",                       // wrong order
		"cluster.summary.read cluster.browse.fetch cluster.terminal.open", // wrong order
		"cluster.summary.read cluster.browse.ws cluster.browse.fetch",     // ws before fetch: wrong order
		"cluster.summary.read cluster.summary.read",                       // duplicate
		"cluster.summary.read cluster.terminal.open cluster.terminal.open",
		"cluster.summary.read cluster.browse.ws cluster.browse.ws", // duplicate ws
		"cluster.summary.read cluster.unknown.token",
		"cluster.summary.read  cluster.terminal.open extra",
	}
	for _, scope := range cases {
		if ValidV2Scope(scope) {
			t.Fatalf("ValidV2Scope(%q) = true, want false", scope)
		}
	}
}

func TestScopeAllowsHelpersOnKnownConstants(t *testing.T) {
	if ScopeAllowsTerminal(SummaryScope) {
		t.Fatal("SummaryScope must not allow terminal")
	}
	if ScopeAllowsBrowse(SummaryScope) {
		t.Fatal("SummaryScope must not allow browse")
	}
	if ScopeAllowsBrowseWS(SummaryScope) {
		t.Fatal("SummaryScope must not allow browse ws")
	}
	if !ScopeAllowsTerminal(SummaryTerminalScope) {
		t.Fatal("SummaryTerminalScope must allow terminal")
	}
	if ScopeAllowsBrowse(SummaryTerminalScope) {
		t.Fatal("SummaryTerminalScope must not allow browse")
	}
	if ScopeAllowsBrowseWS(SummaryTerminalScope) {
		t.Fatal("SummaryTerminalScope must not allow browse ws")
	}
}

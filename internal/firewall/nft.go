package firewall

import "strings"

// nftPolicy only tells an empty ruleset (nothing filters incoming
// traffic) from one with input rules, which portop does not evaluate.
func nftPolicy(ruleset string) Policy {
	if strings.Contains(ruleset, "hook input") {
		return unknownPolicy{}
	}
	return allowAll{}
}

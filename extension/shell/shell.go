package shell

import nova "github.com/laoin114514/NovaBot"

func Parse(s string) []string {
	return nova.ParseShell(s)
}

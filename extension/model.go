package extension

import nova "github.com/laoin114514/NovaBot"

// PrefixModel is model of nova.PrefixRule
type PrefixModel struct {
	Prefix string `nova:"prefix"`
	Args   string `nova:"args"`
}

// SuffixModel is model of nova.SuffixRule
type SuffixModel struct {
	Suffix string `nova:"suffix"`
	Args   string `nova:"args"`
}

// CommandModel is model of nova.CommandRule
type CommandModel struct {
	Command string `nova:"command"`
	Args    string `nova:"args"`
}

// KeywordModel is model of nova.KeywordRule
type KeywordModel struct {
	Keyword string `nova:"keyword"`
}

// FullMatchModel is model of nova.FullMatchRule
type FullMatchModel struct {
	Matched string `nova:"matched"`
}

// RegexModel is model of nova.RegexRule
type RegexModel struct {
	Matched []string `nova:"regex_matched"`
}

// PatternModel is model of nova.PatternRule
type PatternModel struct {
	Matched []nova.PatternParsed `nova:"pattern_matched"`
}

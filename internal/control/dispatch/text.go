package dispatch

import (
	"strings"
	"unicode"
)

// MaxTitle bounds a task title taken from a chat message, in runes.
const MaxTitle = 120

// WantsRussian says whether the fixed chat texts are Russian: the tenant's chat language when it
// names one ("ru"; any other code gets English, the only other set of fixed texts), otherwise the
// language of the message (`auto`).
func WantsRussian(chat, msg string) bool {
	switch chat {
	case "", "auto":
		return IsRussian(msg)
	}
	return chat == "ru"
}

// IsRussian reports whether the message has Cyrillic letters. Chat language is "auto"
// (design section 17): a reply follows the message, English is the default, and a
// Russian message with English technical terms still gets a Russian reply.
func IsRussian(s string) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.Is(unicode.Cyrillic, r) }) >= 0
}

// Title is the first line of a message, cut to MaxTitle runes.
func Title(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	line = strings.TrimSpace(line)
	if r := []rune(line); len(r) > MaxTitle {
		line = string(r[:MaxTitle-1]) + "…"
	}
	if line == "" {
		return "(empty message)"
	}
	return line
}

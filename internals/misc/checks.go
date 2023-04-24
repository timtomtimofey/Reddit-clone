package misc

import (
	"unicode"
	"unicode/utf8"
)

// str expected to have at least one rune in it
func IsBorderSpace(str string) bool {
	last, _ := utf8.DecodeLastRune([]byte(str))
	first, _ := utf8.DecodeRune([]byte(str))
	return unicode.IsSpace(last) || unicode.IsSpace(first)
}

func IsValidUsername(str string) bool {
	for _, c := range str {
		if !(unicode.IsDigit(c) || c == '-' || c == '_' || (unicode.IsLetter(c) && c < unicode.MaxASCII)) {
			return false
		}
	}
	return true
}

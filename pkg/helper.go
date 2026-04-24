package pkg

import (
	"strings"

	"github.com/google/uuid"
)

func GenerateUID() string {
	id := uuid.New()
	return id.String()
}

func MaskString(s string, visibleStart, visibleEnd int) string {
	runes := []rune(s)
	length := len(runes)

	// Kalau string terlalu pendek, sembunyikan semua
	if length <= visibleStart+visibleEnd {
		return strings.Repeat("*", length)
	}

	masked := make([]rune, length)
	for i, r := range runes {
		if i < visibleStart || i >= length-visibleEnd {
			masked[i] = r
		} else {
			masked[i] = '*'
		}
	}
	return string(masked)
}

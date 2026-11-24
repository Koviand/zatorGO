package config

import (
	"fmt"
	"strings"
)

// ValidateSourceSafe проверяет, что текст config безопасен для `source` shell-скриптами
// zapret2: только комментарии, пустые строки и присваивания NAME=…;
// многострочные значения в кавычках не допускают «выхода» в команды;
// запрещены $(...), backtick и shell-метасимволы (;|&<>(){}!?~*) вне кавычек.
func ValidateSourceSafe(content string) error {
	if strings.ContainsRune(content, 0) {
		return fmt.Errorf("config: запрещены NUL-байты")
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	inDQ := false
	for i, line := range strings.Split(content, "\n") {
		lineNo := i + 1
		if strings.Contains(line, "$(") || strings.Contains(line, "`") {
			return fmt.Errorf("config: строка %d: запрещены подстановки $(...) и backtick", lineNo)
		}
		if inDQ {
			closed, clean, err := doubleQuoteClose(line)
			if err != nil {
				return fmt.Errorf("config: строка %d: %w", lineNo, err)
			}
			if closed {
				if !clean {
					return fmt.Errorf("config: строка %d: код после закрывающей кавычки", lineNo)
				}
				inDQ = false
			}
			continue
		}
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		m := assignRe.FindStringSubmatch(line)
		if m == nil {
			return fmt.Errorf("config: строка %d: ожидается присваивание NAME=... или комментарий", lineNo)
		}
		rest := m[2]
		opens, closesClean, err := assignmentQuoteState(rest)
		if err != nil {
			return fmt.Errorf("config: строка %d: %w", lineNo, err)
		}
		if opens {
			inDQ = true
			continue
		}
		if !closesClean {
			return fmt.Errorf("config: строка %d: код после закрывающей кавычки", lineNo)
		}
	}
	if inDQ {
		return fmt.Errorf("config: незакрытая кавычка многострочного значения")
	}
	return nil
}

// assignmentQuoteState разбирает RHS присваивания.
// opens — значение открыло многострочные кавычки;
// closesClean — однострочное значение корректно (нет хвоста после кавычки / нет ;|& вне кавычек).
func assignmentQuoteState(rest string) (opens, closesClean bool, err error) {
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return false, true, nil
	}
	if rest[0] != '"' {
		if hasUnquotedShellMeta(rest) {
			return false, false, fmt.Errorf("опасные shell-метасимволы (;|&<>(){}!?~*) вне кавычек")
		}
		return false, true, nil
	}
	// quoted value starting at rest[0]
	idx := closingQuote(rest[1:])
	if idx < 0 {
		// "… без закрытия на этой строке → multiline
		return true, true, nil
	}
	// rest = "VALUE" optional-junk
	after := rest[1+idx+1:]
	if strings.TrimSpace(stripTrailingComment(after)) != "" {
		return false, false, fmt.Errorf("код после закрывающей кавычки")
	}
	return false, true, nil
}

// doubleQuoteClose — продолжение многострочного "…".
func doubleQuoteClose(line string) (closed, clean bool, err error) {
	idx := closingQuote(line)
	if idx < 0 {
		return false, true, nil
	}
	after := line[idx+1:]
	if strings.TrimSpace(stripTrailingComment(after)) != "" {
		return true, false, fmt.Errorf("код после закрывающей кавычки")
	}
	return true, true, nil
}

// stripTrailingComment снимает хвостовой ` #…` вне кавычек (уже после закрытия).
func stripTrailingComment(s string) string {
	if i := strings.IndexByte(s, '#'); i >= 0 {
		return s[:i]
	}
	return s
}

func hasUnquotedShellMeta(s string) bool {
	inDQ, inSQ, esc := false, false, false
	for _, r := range s {
		if esc {
			esc = false
			continue
		}
		if r == '\\' && inDQ {
			esc = true
			continue
		}
		if r == '"' && !inSQ {
			inDQ = !inDQ
			continue
		}
		if r == '\'' && !inDQ {
			inSQ = !inSQ
			continue
		}
		if inDQ || inSQ {
			continue
		}
		switch r {
		case ';', '|', '&', '>', '<', '(', ')', '{', '}', '!', '?', '~', '*':
			return true
		}
	}
	return false
}

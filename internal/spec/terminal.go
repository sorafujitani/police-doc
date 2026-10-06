package spec

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// normalizeTerminalOutput removes ECMA-48 control sequences before metadata is
// interpreted. It preserves text and layout, not terminal cursor/overwrite
// effects. Control-string payloads (URLs, titles, etc.) are never evidence.
// Both ESC-prefixed and C1 forms are accepted, without treating UTF-8 continuation
// bytes as controls. Incomplete/malformed sequences fail rather than expose their
// payload as flags, commands or versions.
func normalizeTerminalOutput(output string) (string, error) {
	const (
		text = iota
		escape
		escapeIntermediate
		csi
		csiIntermediate
		osc
		controlString
	)
	state, stringEscape := text, false
	var plain strings.Builder
	plain.Grow(len(output))
	for i := 0; i < len(output); {
		char, size := utf8.DecodeRuneInString(output[i:])
		if char == utf8.RuneError && size == 1 {
			char = rune(output[i]) // Also accept legacy single-byte C1 controls.
		}
		start := i
		i += size
		if state == osc || state == controlString {
			if char == 0x9c || (stringEscape && char == '\\') || (state == osc && char == '\a') {
				state, stringEscape = text, false
			} else {
				stringEscape = char == '\x1b'
			}
			continue
		}
		switch state {
		case text:
			switch char {
			case '\x1b':
				state = escape
			case 0x9b:
				state = csi
			case 0x9d:
				state = osc
			case 0x90, 0x98, 0x9e, 0x9f: // DCS, SOS, PM, APC
				state = controlString
			default:
				if char == '\t' || char == '\n' || char == '\r' || (char >= 0x20 && !(char >= 0x7f && char <= 0x9f)) {
					plain.WriteString(output[start:i])
				}
			}
		case escape:
			switch char {
			case '[':
				state = csi
			case ']':
				state = osc
			case 'P', 'X', '^', '_':
				state = controlString
			default:
				if char >= 0x20 && char <= 0x2f {
					state = escapeIntermediate
				} else if char >= 0x30 && char <= 0x7e {
					state = text
				} else {
					return "", fmt.Errorf("invalid terminal escape at byte %d", start)
				}
			}
		case escapeIntermediate:
			if char >= 0x30 && char <= 0x7e {
				state = text
			} else if char < 0x20 || char > 0x2f {
				return "", fmt.Errorf("invalid terminal escape at byte %d", start)
			}
		case csi, csiIntermediate:
			if char >= 0x40 && char <= 0x7e {
				state = text
			} else if char >= 0x20 && char <= 0x2f {
				state = csiIntermediate
			} else if state != csi || char < 0x30 || char > 0x3f {
				return "", fmt.Errorf("invalid terminal CSI at byte %d", start)
			}
		}
	}
	if state != text {
		return "", fmt.Errorf("unterminated terminal control sequence")
	}
	return plain.String(), nil
}

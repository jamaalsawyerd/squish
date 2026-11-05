package tui

import (
	"os"
	"regexp"
	"strings"
)

var windowsPath = regexp.MustCompile(`^([A-Za-z]):[\\/](.*)$`)

// parsePaths understands what terminals send when files are dragged in or
// paths are pasted: one or many, with quotes or backslash escapes.
func parsePaths(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r", "\n"), "\n") {
		out = append(out, splitLine(strings.TrimSpace(line))...)
	}
	for i, p := range out {
		out[i] = normalizePath(p)
	}
	return out
}

func splitLine(s string) []string {
	var (
		out     []string
		cur     strings.Builder
		quote   rune
		started bool
	)
	flush := func() {
		if started {
			out = append(out, cur.String())
			cur.Reset()
			started = false
		}
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == '\\' && i+1 < len(rs) && isEscapable(rs[i+1]):
			i++
			cur.WriteRune(rs[i])
			started = true
		case r == ' ' || r == '\t':
			flush()
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	flush()
	return out
}

// isEscapable keeps Windows path separators intact, where a backslash is
// followed by an ordinary letter.
func isEscapable(r rune) bool {
	return strings.ContainsRune(` '"()&[]{}$!;\`, r)
}

// normalizePath expands ~ and maps Windows paths to their WSL location, since
// dragging from Explorer into a WSL terminal can send either form.
func normalizePath(p string) string {
	if home, err := os.UserHomeDir(); err == nil {
		if p == "~" {
			return home
		}
		if rest, ok := strings.CutPrefix(p, "~/"); ok {
			return home + "/" + rest
		}
	}
	if m := windowsPath.FindStringSubmatch(p); m != nil && isWSL() {
		return "/mnt/" + strings.ToLower(m[1]) + "/" + strings.ReplaceAll(m[2], `\`, "/")
	}
	return p
}

func isWSL() bool {
	return os.Getenv("WSL_DISTRO_NAME") != ""
}

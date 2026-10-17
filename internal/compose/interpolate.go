package compose

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// projectEnv returns the variables Compose would substitute in dir's
// files: the .env file next to them, overridden by the process
// environment, as docker compose does.
func projectEnv(dir string) (map[string]string, error) {
	env := map[string]string{}
	f, err := os.Open(filepath.Join(dir, ".env"))
	switch {
	case err == nil:
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			line = strings.TrimPrefix(line, "export ")
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
				value = value[1 : len(value)-1]
			} else if i := strings.Index(value, " #"); i >= 0 {
				value = strings.TrimSpace(value[:i])
			}
			env[strings.TrimSpace(key)] = value
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf(".env: %w", err)
		}
	case !os.IsNotExist(err):
		return nil, err
	}
	for _, kv := range os.Environ() {
		if key, value, ok := strings.Cut(kv, "="); ok {
			env[key] = value
		}
	}
	return env, nil
}

// interpolateNode substitutes variables in every scalar value of a parsed
// YAML document, so "${WEB_PORT:-8080}:80" reaches the port parser as
// "8080:80". Keys are left alone, as in Compose.
func interpolateNode(n *yaml.Node, env map[string]string) error {
	if n.Kind == yaml.ScalarNode {
		v, err := interpolate(n.Value, env)
		if err != nil {
			return fmt.Errorf("line %d: %w", n.Line, err)
		}
		n.Value = v
		return nil
	}
	for i, child := range n.Content {
		if n.Kind == yaml.MappingNode && i%2 == 0 {
			continue
		}
		if err := interpolateNode(child, env); err != nil {
			return err
		}
	}
	return nil
}

// interpolate implements Compose's variable syntax: $VAR, ${VAR},
// ${VAR:-default}, ${VAR-default}, ${VAR:?error}, ${VAR?error},
// ${VAR:+replacement}, ${VAR+replacement} and $$ for a literal $.
func interpolate(s string, env map[string]string) (string, error) {
	if !strings.Contains(s, "$") {
		return s, nil
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '$' {
			b.WriteByte(s[i])
			continue
		}
		switch {
		case i+1 < len(s) && s[i+1] == '$':
			b.WriteByte('$')
			i++
		case i+1 < len(s) && s[i+1] == '{':
			end := closingBrace(s, i+2)
			if end < 0 {
				return "", fmt.Errorf("unterminated variable in %q", s)
			}
			v, err := expand(s[i+2:end], env)
			if err != nil {
				return "", err
			}
			b.WriteString(v)
			i = end
		default:
			j := i + 1
			for j < len(s) && isNameByte(s[j], j == i+1) {
				j++
			}
			if j == i+1 {
				b.WriteByte('$')
				continue
			}
			b.WriteString(env[s[i+1:j]])
			i = j - 1
		}
	}
	return b.String(), nil
}

func closingBrace(s string, from int) int {
	depth := 1
	for i := from; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func expand(expr string, env map[string]string) (string, error) {
	n := 0
	for n < len(expr) && isNameByte(expr[n], n == 0) {
		n++
	}
	name, op := expr[:n], expr[n:]
	if name == "" {
		return "", fmt.Errorf("invalid variable ${%s}", expr)
	}
	value, set := env[name]
	nonEmpty := set && value != ""
	for _, form := range []string{":-", "-", ":?", "?", ":+", "+"} {
		if !strings.HasPrefix(op, form) {
			continue
		}
		arg, err := interpolate(op[len(form):], env)
		if err != nil {
			return "", err
		}
		ok := set
		if form[0] == ':' {
			ok = nonEmpty
		}
		switch form[len(form)-1] {
		case '-':
			if ok {
				return value, nil
			}
			return arg, nil
		case '?':
			if ok {
				return value, nil
			}
			if arg == "" {
				arg = "is not set"
			}
			return "", fmt.Errorf("%s: %s", name, arg)
		default: // '+'
			if ok {
				return arg, nil
			}
			return "", nil
		}
	}
	if op != "" {
		return "", fmt.Errorf("invalid variable ${%s}", expr)
	}
	return value, nil
}

func isNameByte(c byte, first bool) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (!first && c >= '0' && c <= '9')
}

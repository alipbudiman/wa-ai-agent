package skilldev

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type MarkdownWriter struct {
	builder strings.Builder
}

func NewMarkdownWriter() *MarkdownWriter {
	return &MarkdownWriter{}
}

func (m *MarkdownWriter) H1(text string) *MarkdownWriter {
	m.builder.WriteString(fmt.Sprintf("# %s\n\n", text))
	return m
}

func (m *MarkdownWriter) H2(text string) *MarkdownWriter {
	m.builder.WriteString(fmt.Sprintf("## %s\n\n", text))
	return m
}

func (m *MarkdownWriter) H3(text string) *MarkdownWriter {
	m.builder.WriteString(fmt.Sprintf("### %s\n\n", text))
	return m
}

func (m *MarkdownWriter) Text(text string) *MarkdownWriter {
	fmt.Fprintf(&m.builder, "%s\n\n", text)
	return m
}

func (m *MarkdownWriter) BulletList(items []string) *MarkdownWriter {
	for _, item := range items {
		fmt.Fprintf(&m.builder, "- %s\n", item)
	}
	m.builder.WriteString("\n")
	return m
}

func (m *MarkdownWriter) Table(headers []string, rows [][]string) *MarkdownWriter {
	// header
	m.builder.WriteString("| " + strings.Join(headers, " | ") + " |\n")

	// separator
	separators := make([]string, len(headers))
	for i := range separators {
		separators[i] = "---"
	}
	m.builder.WriteString("| " + strings.Join(separators, " | ") + " |\n")

	// rows
	for _, row := range rows {
		m.builder.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	m.builder.WriteString("\n")
	return m
}

func (m *MarkdownWriter) CodeBlock(lang, code string) *MarkdownWriter {
	fmt.Fprintf(&m.builder, "```%s\n%s\n```\n\n", lang, code)
	return m
}

func (m *MarkdownWriter) Save(filename string) error {
	return os.WriteFile(filename, []byte(m.builder.String()), 0644)
}

func ReadSkills(file_name string) string {
	fileName := strings.TrimSpace(file_name)
	if fileName == "" {
		return ""
	}

	paths := make([]string, 0, 16)
	seen := make(map[string]struct{}, 16)

	addPath := func(path string) {
		if strings.TrimSpace(path) == "" {
			return
		}

		normalized := filepath.Clean(path)
		if _, ok := seen[normalized]; ok {
			return
		}

		seen[normalized] = struct{}{}
		paths = append(paths, normalized)
	}

	addPath(fileName)

	if cwd, err := os.Getwd(); err == nil {
		addPath(filepath.Join(cwd, fileName))
		addPath(filepath.Join(cwd, "skills", fileName))
	}

	if _, currentFile, _, ok := runtime.Caller(0); ok {
		dir := filepath.Dir(currentFile)
		for {
			addPath(filepath.Join(dir, fileName))
			addPath(filepath.Join(dir, "skills", fileName))

			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		trimmed := strings.TrimSpace(string(content))
		if trimmed != "" {
			return trimmed
		}
	}

	return ""
}

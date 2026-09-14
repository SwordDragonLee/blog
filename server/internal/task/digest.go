package task

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"blog/server/internal/analyzer"
	"blog/server/internal/llm"
)

const (
	readmeLimitBytes = 8 * 1024 // README 摘要截断大小
)

// buildDigest 组装供 LLM 分析的仓库材料：基本信息、语言统计、目录树、
// README 前 8KB、采样文件全文（按 path+content 拼装）。
func buildDigest(info *analyzer.RepoInfo, det *analyzer.Detection, files []analyzer.SampledFile) string {
	var b strings.Builder

	b.WriteString(llm.DigestSection("仓库基本信息", fmt.Sprintf(
		"URL: %s\n默认分支: %s\n最近提交: %s",
		info.URL, info.DefaultBranch, info.HeadSubject)))

	if len(det.Languages) > 0 {
		lines := make([]string, 0, len(det.Languages))
		for _, ls := range det.Languages {
			lines = append(lines, fmt.Sprintf("%s: %d 个文件", ls.Language, ls.Files))
		}
		b.WriteString(llm.DigestSection("语言统计", strings.Join(lines, "\n")))
	}
	if len(det.TechStack) > 0 {
		b.WriteString(llm.DigestSection("技术栈线索（静态探测，供参考）", strings.Join(det.TechStack, "、")))
	}
	if len(det.Tree) > 0 {
		b.WriteString(llm.DigestSection("目录树", strings.Join(det.Tree, "\n")))
	}
	if readme := readReadme(info.Dir, readmeLimitBytes); readme != "" {
		b.WriteString(llm.DigestSection("README（前 8KB）", readme))
	}

	for _, f := range files {
		fmt.Fprintf(&b, "### 采样文件: %s\n```%s\n%s\n```\n\n", f.Path, langOf(f.Path), f.Content)
	}
	return b.String()
}

// readReadme 读取仓库根目录 README（忽略大小写），超出 limit 截断。
func readReadme(dir string, limit int64) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(strings.ToLower(e.Name()), "readme") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return ""
		}
		if int64(len(data)) > limit {
			data = data[:limit]
		}
		return string(data)
	}
	return ""
}

// langOf 由文件扩展名给出 markdown 代码块语言标识。
func langOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".js", ".jsx":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".php":
		return "php"
	case ".rb":
		return "ruby"
	case ".cs":
		return "csharp"
	case ".c", ".h":
		return "c"
	case ".cpp", ".cc", ".hpp":
		return "cpp"
	case ".kt":
		return "kotlin"
	case ".swift":
		return "swift"
	case ".scala":
		return "scala"
	case ".vue":
		return "vue"
	case ".svelte":
		return "svelte"
	case ".sql":
		return "sql"
	case ".sh":
		return "bash"
	case ".proto":
		return "protobuf"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".json":
		return "json"
	case ".xml", ".html":
		return "xml"
	case ".md":
		return "markdown"
	case ".mod":
		return "go"
	}
	return ""
}

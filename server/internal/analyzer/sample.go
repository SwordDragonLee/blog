package analyzer

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SampledFile 是一个被采样的代码文件。
type SampledFile struct {
	Path    string
	Content string
}

// 优先采样的文件名 → 加分。
var importantNames = map[string]int{
	"main.go": 60, "app.py": 60, "main.py": 60, "server.go": 50, "app.go": 50,
	"server.py": 50, "main.ts": 40, "index.ts": 40, "index.js": 40, "mod.go": 40,
	"go.mod": 45, "package.json": 45, "requirements.txt": 35, "Cargo.toml": 40,
	"pom.xml": 35, "build.gradle": 30, "Makefile": 25, "Dockerfile": 30,
	"docker-compose.yml": 30, "docker-compose.yaml": 30, ".env.example": 15,
	"router.go": 35, "routes.go": 35, "handler.go": 25, "service.go": 25,
	"models.go": 25, "config.go": 25, "db.go": 20, "wire.go": 30,
}

// 路径前缀加分（可命中的关键代码目录）。
var pathPrefixBonus = []string{"cmd/", "src/", "app/", "internal/", "pkg/", "server/", "api/", "lib/", "gateway/", "services/"}

// 采样的扩展名白名单（源码 + 关键工程文件）。
var sampleExts = map[string]bool{
	".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".py": true, ".java": true, ".rs": true, ".php": true, ".rb": true,
	".cs": true, ".c": true, ".h": true, ".cpp": true, ".hpp": true,
	".kt": true, ".swift": true, ".scala": true, ".vue": true, ".svelte": true,
	".sql": true, ".sh": true, ".proto": true, ".yaml": true, ".yml": true,
	".toml": true, ".json": true, ".md": true, ".mod": true, ".gradle": true,
	".xml": true, ".cfg": true, ".ini": true,
}

// Sample 按重要性打分排序，在文件数与总字节预算内采样核心代码文件。
func Sample(root string, det *Detection, maxFiles, maxFileKB, totalBudgetKB int) []SampledFile {
	type cand struct {
		path  string
		score int
	}
	var cands []cand
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel = filepath.ToSlash(rel)
		if strings.Count(rel, "/") > 6 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Size() == 0 || info.Size() > 200*1024 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !sampleExts[ext] {
			return nil
		}

		score := 0
		if bonus, ok := importantNames[d.Name()]; ok {
			score += bonus
		}
		for _, p := range pathPrefixBonus {
			if strings.HasPrefix(rel, p) {
				score += 30
				break
			}
		}
		if strings.Contains(strings.ToLower(rel), "test") {
			score -= 10
		}
		if det != nil && len(det.Languages) > 0 && ext == langExt(det.Languages[0].Language) {
			score += 15
		}
		if info.Size() > 100*1024 {
			score -= 15
		}
		if score > 0 {
			cands = append(cands, cand{path: rel, score: score})
		}
		return nil
	})

	sort.Slice(cands, func(i, j int) bool { return cands[i].score > cands[j].score })

	var files []SampledFile
	budget := totalBudgetKB * 1024
	for _, c := range cands {
		if len(files) >= maxFiles || budget <= 0 {
			break
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(c.path)))
		if err != nil {
			continue
		}
		if len(data) > maxFileKB*1024 {
			data = data[:maxFileKB*1024]
			if i := bytesLastIndexByte(data, '\n'); i > 0 {
				data = data[:i]
			}
			data = append(data, []byte("\n/* ...(已截断) */")...)
		}
		budget -= len(data)
		files = append(files, SampledFile{Path: c.path, Content: string(data)})
	}
	return files
}

func bytesLastIndexByte(b []byte, c byte) int {
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == c {
			return i
		}
	}
	return -1
}

// langExt 语言名 → 代表性扩展名（用于主语言加分）。
func langExt(lang string) string {
	switch lang {
	case "Go":
		return ".go"
	case "JavaScript":
		return ".js"
	case "TypeScript":
		return ".ts"
	case "Python":
		return ".py"
	case "Java":
		return ".java"
	case "Rust":
		return ".rs"
	case "PHP":
		return ".php"
	case "Ruby":
		return ".rb"
	case "C#":
		return ".cs"
	case "C":
		return ".c"
	case "C++":
		return ".cpp"
	case "Vue":
		return ".vue"
	}
	return ""
}

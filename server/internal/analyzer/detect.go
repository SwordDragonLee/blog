package analyzer

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// LangStat 是语言文件数统计。
type LangStat struct {
	Language string `json:"language"`
	Files    int    `json:"files"`
}

// Detection 是静态探测结果。
type Detection struct {
	TechStack []string
	Languages []LangStat
	Tree      []string // 目录树摘要（相对路径，目录以 / 结尾）
}

// extLang 常见源码扩展名 → 语言名。
var extLang = map[string]string{
	".go": "Go", ".js": "JavaScript", ".jsx": "JavaScript", ".ts": "TypeScript",
	".tsx": "TypeScript", ".py": "Python", ".java": "Java", ".rs": "Rust",
	".php": "PHP", ".rb": "Ruby", ".cs": "C#", ".c": "C", ".h": "C",
	".cpp": "C++", ".cc": "C++", ".hpp": "C++", ".kt": "Kotlin", ".swift": "Swift",
	".scala": "Scala", ".vue": "Vue", ".svelte": "Svelte", ".lua": "Lua",
	".sql": "SQL", ".sh": "Shell", ".proto": "Protobuf",
}

// featureFiles 特征文件 → 技术栈标签。
var featureFiles = map[string][]string{
	"go.mod":             {"Go Modules"},
	"package.json":       {"npm"},
	"requirements.txt":   {"pip"},
	"pyproject.toml":     {"Python"},
	"Cargo.toml":         {"Cargo"},
	"pom.xml":            {"Maven"},
	"build.gradle":       {"Gradle"},
	"composer.json":      {"Composer"},
	"Gemfile":            {"Bundler"},
	"docker-compose.yml": {"Docker Compose"},
	"Dockerfile":         {"Docker"},
	"Makefile":           {"Make"},
	"nginx.conf":         {"Nginx"},
}

// goFrameworks go.mod 依赖路径片段 → 技术栈名。
var goFrameworks = []struct {
	match string
	name  string
}{
	{"gin-gonic", "Gin"}, {"gorm.io", "GORM"}, {"gofiber", "Fiber"},
	{"labstack/echo", "Echo"}, {"astaxie/beego", "Beego"}, {"go-chi", "chi"},
	{"go-redis", "Redis"}, {"go-sql-driver", "MySQL"}, {"mongo-driver", "MongoDB"},
	{"go-kafka", "Kafka"}, {"segmentio/kafka-go", "Kafka"}, {"grpc", "gRPC"},
	{"spf13/viper", "Viper"}, {"spf13/cobra", "Cobra"}, {"uber-go/zap", "zap"},
	{"go.uber.org/fx", "Uber fx"}, {"gorilla", "Gorilla"}, {"testify", "testify"},
	{"gqlgen", "GraphQL"}, {"elastic", "Elasticsearch"}, {"nats", "NATS"},
}

// jsFrameworks package.json 依赖名 → 技术栈名。
var jsFrameworks = map[string]string{
	"react": "React", "vue": "Vue", "next": "Next.js", "nuxt": "Nuxt",
	"svelte": "Svelte", "vite": "Vite", "webpack": "webpack", "antd": "Ant Design",
	"element-plus": "Element Plus", "express": "Express", "koa": "Koa",
	"@nestjs/core": "NestJS", "egg": "Egg", "typescript": "TypeScript",
	"electron": "Electron", "tailwindcss": "Tailwind CSS", "prisma": "Prisma",
	"mongoose": "Mongoose", "socket.io": "Socket.IO", "rxjs": "RxJS",
	"mobx": "MobX", "redux": "Redux", "jest": "Jest", "vitest": "Vitest",
}

// pyFrameworks requirements/pyproject 片段 → 技术栈名。
var pyFrameworks = []struct {
	match string
	name  string
}{
	{"django", "Django"}, {"flask", "Flask"}, {"fastapi", "FastAPI"},
	{"tornado", "Tornado"}, {"numpy", "NumPy"}, {"pandas", "pandas"},
	{"torch", "PyTorch"}, {"tensorflow", "TensorFlow"}, {"celery", "Celery"},
	{"sqlalchemy", "SQLAlchemy"}, {"scrapy", "Scrapy"},
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, "dist": true,
	"build": true, "target": true, "out": true, "bin": true, "obj": true,
	".next": true, ".nuxt": true, "coverage": true, "__pycache__": true,
	".venv": true, "venv": true, ".idea": true, ".vscode": true,
	"bower_components": true, ".gradle": true, ".mvn": true,
}

// Detect 扫描仓库根目录：统计语言、识别特征文件与已知框架。
func Detect(root string) (*Detection, error) {
	det := &Detection{}
	langCount := map[string]int{}
	features := map[string]bool{}
	treeEntries := []string{}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // 单个条目失败不中断扫描
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		depth := strings.Count(rel, "/")
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return fs.SkipDir
			}
			if depth <= 2 && len(treeEntries) < 120 {
				treeEntries = append(treeEntries, rel+"/")
			}
			return nil
		}
		if depth > 6 || len(treeEntries) > 5000 {
			return nil
		}
		if depth <= 1 && len(treeEntries) < 120 {
			treeEntries = append(treeEntries, rel)
		}

		base := d.Name()
		features[base] = true
		if tags, ok := featureFiles[base]; ok {
			det.TechStack = append(det.TechStack, tags...)
		}
		if lang, ok := extLang[strings.ToLower(filepath.Ext(base))]; ok {
			langCount[lang]++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	det.Tree = treeEntries
	for lang := range langCount {
		det.Languages = append(det.Languages, LangStat{Language: lang, Files: langCount[lang]})
	}
	sort.Slice(det.Languages, func(i, j int) bool { return det.Languages[i].Files > det.Languages[j].Files })
	if len(det.Languages) > 6 {
		det.Languages = det.Languages[:6]
	}

	det.TechStack = mergeUnique(det.TechStack, parseGoMod(filepath.Join(root, "go.mod")))
	det.TechStack = mergeUnique(det.TechStack, parsePackageJSON(filepath.Join(root, "package.json")))
	det.TechStack = mergeUnique(det.TechStack, parsePythonDeps(root, features))

	// 主语言本身也入栈
	for _, ls := range det.Languages {
		det.TechStack = append([]string{ls.Language}, det.TechStack...)
		break
	}
	det.TechStack = uniqueNonEmpty(det.TechStack)
	if len(det.TechStack) > 14 {
		det.TechStack = det.TechStack[:14]
	}
	return det, nil
}

func parseGoMod(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		for _, fw := range goFrameworks {
			if strings.Contains(line, fw.match) {
				out = append(out, fw.name)
			}
		}
	}
	return out
}

func parsePackageJSON(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return nil
	}
	var out []string
	for dep := range pkg.Dependencies {
		if name, ok := jsFrameworks[dep]; ok {
			out = append(out, name)
		}
	}
	for dep := range pkg.DevDependencies {
		if name, ok := jsFrameworks[dep]; ok {
			out = append(out, name)
		}
	}
	return out
}

func parsePythonDeps(root string, features map[string]bool) []string {
	var text string
	if features["requirements.txt"] {
		if data, err := os.ReadFile(filepath.Join(root, "requirements.txt")); err == nil {
			text = string(data)
		}
	}
	if text == "" && features["pyproject.toml"] {
		if data, err := os.ReadFile(filepath.Join(root, "pyproject.toml")); err == nil {
			text = string(data)
		}
	}
	if text == "" {
		return nil
	}
	text = strings.ToLower(text)
	var out []string
	for _, fw := range pyFrameworks {
		if strings.Contains(text, fw.match) {
			out = append(out, fw.name)
		}
	}
	return out
}

func mergeUnique(dst, src []string) []string { return append(dst, src...) }

func uniqueNonEmpty(in []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

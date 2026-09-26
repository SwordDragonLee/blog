package service

import (
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// buildFixtureDoc 构造综合测试文档：h1 + 四个 h2 节（含 h3）、短代码块（内含 # 注释行）、
// 单行 HTML 注释、figure 占位符、表格、列表（含缩进续行）。各节正文用 filler 撑到
// ~300 rune，配合 chunkSize=300 让每节独立成块（相邻合并超 target），标题路径与
// 表格/代码归属的断言因此是确定性的。
func buildFixtureDoc() string {
	var b strings.Builder
	b.WriteString("# LangChain 优雅代码赏析\n\n")
	b.WriteString("## 一、懒加载\n\n" + filler("LAZY", 28) + "。\n\n")
	b.WriteString("```python\n# 注释行不应触发分节\nfrom core import lazy\nlazy_import()\n```\n\n")
	b.WriteString("## 二、deprecated 装饰器\n\n### FieldInfo 支持\n\n" + filler("DEP", 28) + "。\n\n")
	b.WriteString("## 三、SSRF 防护\n\n" + filler("SSRF", 24) + "。\n\n")
	b.WriteString("| 方法 | 说明 |\n| --- | --- |\n| 校验 | 逐个 IP 检查 |\n| 监控 | 记录放行日志 |\n\n")
	b.WriteString("## 四、收尾\n\n" + filler("END", 12) + "。\n\n")
	b.WriteString("- ITEM-01 列表首项\n- ITEM-02 列表次项\n  CONT-02 缩进续行\n- ITEM-03 列表末项\n\n结尾正文一段。\n")
	return b.String()
}

// filler 生成 n 段「标记+序号+正文」拼接的单行长文本，每段约 11 rune。
func filler(marker string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(marker)
		b.WriteString(strconv.Itoa(i))
		b.WriteString("一句测试正文")
	}
	return b.String()
}

// fenceParity 块内围栏标记数（按子串计），必须为偶数——奇数即代码块被切断。
func fenceParity(content string) int { return strings.Count(content, "```") % 2 }

// extractOrder 按出现顺序取出 marker 数字序列（跨块拼接后应严格递增，验证不重不漏）。
func extractOrder(contents []string, re *regexp.Regexp) []int {
	var order []int
	for _, c := range contents {
		for _, m := range re.FindAllStringSubmatch(c, -1) {
			n, _ := strconv.Atoi(m[1])
			order = append(order, n)
		}
	}
	return order
}

func assertStrictlyIncreasing(t *testing.T, order []int, what string) {
	t.Helper()
	for i := 1; i < len(order); i++ {
		if order[i] <= order[i-1] {
			t.Fatalf("%s 序列在 %d 处非严格递增: %v（切片内容重复或乱序）", what, i, order)
		}
	}
}

func TestChunkMarkdownFencesBalanced(t *testing.T) {
	doc := buildFixtureDoc()
	for _, size := range []int{200, 300, 500, 1000} {
		chunks := chunkMarkdown(doc, size, size/8)
		if len(chunks) == 0 {
			t.Fatalf("size=%d 切出零块", size)
		}
		for i, c := range chunks {
			if fenceParity(c.Content) != 0 {
				t.Errorf("size=%d 块 %d 围栏数为奇数（代码块被切断）", size, i)
			}
		}
	}
}

func TestChunkMarkdownStartsAtLineBoundary(t *testing.T) {
	doc := buildFixtureDoc()
	lines := map[string]bool{}
	for _, l := range strings.Split(cleanMarkdownForIndex(doc), "\n") {
		lines[strings.TrimSpace(l)] = true
	}
	for i, c := range chunkMarkdown(doc, 300, 40) {
		first := strings.SplitN(c.Content, "\n", 2)[0]
		if strings.HasPrefix(first, "```") { // 合成开栏行属于合法开头
			continue
		}
		if !lines[strings.TrimSpace(first)] {
			t.Errorf("块 %d 开头不是源文完整行: %q", i, first)
		}
	}
}

func TestChunkMarkdownLengthBounds(t *testing.T) {
	doc := buildFixtureDoc()
	for _, size := range []int{200, 300, 500} {
		hardCap := size * 8 / 5
		for i, c := range chunkMarkdown(doc, size, size/8) {
			if n := runeLen(c.Content); n > hardCap+64 {
				t.Errorf("size=%d 块 %d 长度 %d 超过硬上限 %d（含余量）", size, i, n, hardCap+64)
			}
		}
	}
}

func TestChunkMarkdownTitlePath(t *testing.T) {
	chunks := chunkMarkdown(buildFixtureDoc(), 300, 40)
	tests := []struct{ marker, want string }{
		{"LAZY0", "一、懒加载"},
		{"DEP0", "二、deprecated 装饰器 > FieldInfo 支持"},
		{"SSRF0", "三、SSRF 防护"},
	}
	for _, tc := range tests {
		found := false
		for _, c := range chunks {
			if !strings.Contains(c.Content, tc.marker) {
				continue
			}
			found = true
			if c.TitlePath != tc.want {
				t.Errorf("含 %s 的块路径 = %q，期望 %q", tc.marker, c.TitlePath, tc.want)
			}
		}
		if !found {
			t.Errorf("找不到含 %s 的块", tc.marker)
		}
	}
	for i, c := range chunks { // h1 是文章标题，不进路径；fixture 无文首散段，路径应全覆盖
		if strings.Contains(c.TitlePath, "LangChain 优雅代码赏析") {
			t.Errorf("块 %d 路径含 h1 标题: %q", i, c.TitlePath)
		}
		if c.TitlePath == "" {
			t.Errorf("块 %d 缺标题路径: %.40q", i, c.Content)
		}
	}
}

func TestChunkMarkdownHeadingInsideCodeNotSplit(t *testing.T) {
	for _, c := range chunkMarkdown(buildFixtureDoc(), 300, 40) {
		if !strings.Contains(c.Content, "# 注释行不应触发分节") {
			continue
		}
		// 注释行与其所属代码块 opener、后续代码必须同块（未被当标题切走）
		if !strings.Contains(c.Content, "```python") || !strings.Contains(c.Content, "lazy_import()") {
			t.Errorf("围栏内注释行触发了分节，代码块被拆散")
		}
		return
	}
	t.Fatal("找不到含围栏内注释行的块")
}

func TestChunkMarkdownTableKeptIntact(t *testing.T) {
	for _, c := range chunkMarkdown(buildFixtureDoc(), 300, 40) {
		if !strings.Contains(c.Content, "| 方法 | 说明 |") {
			continue
		}
		// 表头、分隔行、全部数据行必须落在同一块
		for _, row := range []string{"| --- | --- |", "| 校验 |", "| 监控 |"} {
			if !strings.Contains(c.Content, row) {
				t.Errorf("表格被拆散：块内缺 %q", row)
			}
		}
		return
	}
	t.Fatal("找不到含表格的块")
}

func TestChunkMarkdownListSplitAtItemBoundary(t *testing.T) {
	var b strings.Builder
	b.WriteString("# 列表文章\n\n")
	for i := 1; i <= 15; i++ {
		b.WriteString("- ITEM-" + strconv.Itoa(i) + " " + filler("L", 6) + "\n")
		if i == 7 {
			b.WriteString("  CONT-07 缩进续行内容\n") // 续行必须跟随所属条目
		}
	}
	chunks := chunkMarkdown(b.String(), 300, 40)
	if len(chunks) < 2 {
		t.Fatalf("长列表应切出多块，实得 %d", len(chunks))
	}
	for i, c := range chunks {
		first := strings.SplitN(c.Content, "\n", 2)[0]
		if i > 0 && !listTopRe.MatchString(first) {
			t.Errorf("块 %d 开头不是列表条目边界: %q", i, first)
		}
		if strings.Contains(c.Content, "ITEM-07") && !strings.Contains(c.Content, "CONT-07") {
			t.Errorf("列表条目被从续行处切开")
		}
	}
}

func TestChunkMarkdownOversizedFenceWrapped(t *testing.T) {
	var b strings.Builder
	b.WriteString("## 代码节\n\n```python\n")
	for i := 0; i < 60; i++ {
		b.WriteString("value_" + strconv.Itoa(i) + " = compute(i)  # CODE\n")
	}
	b.WriteString("```\n")
	chunks := chunkMarkdown(b.String(), 400, 60)
	if len(chunks) < 2 {
		t.Fatalf("超长代码块应被强制切分，实得 %d 块", len(chunks))
	}
	var withCode []string
	for i, c := range chunks {
		if !strings.Contains(c.Content, "CODE") {
			continue
		}
		withCode = append(withCode, c.Content)
		if !strings.Contains(c.Content, "```python") {
			t.Errorf("代码碎片 %d 缺开栏行（真实或合成都该有）", i)
		}
		if !strings.HasSuffix(c.Content, "```") {
			t.Errorf("代码碎片 %d 未以闭栏行结尾", i)
		}
		if fenceParity(c.Content) != 0 {
			t.Errorf("代码碎片 %d 围栏不配对", i)
		}
	}
	if len(withCode) < 2 {
		t.Fatalf("代码块应被切成多片，实得 %d 片", len(withCode))
	}
	// 代码行跨片必须严格递增：不重复（无 overlap）、不丢行
	order := extractOrder(withCode, regexp.MustCompile(`value_(\d+)`))
	if len(order) != 60 {
		t.Fatalf("代码行应 60 行全保留，实得 %d 行", len(order))
	}
	assertStrictlyIncreasing(t, order, "代码行")
}

func TestChunkMarkdownOversizedTableHeaderReemitted(t *testing.T) {
	var b strings.Builder
	b.WriteString("## 表格节\n\n| 名称 | 说明 |\n| --- | --- |\n")
	for i := 0; i < 30; i++ {
		b.WriteString("| ROW-" + strconv.Itoa(i) + " | " + filler("R", 5) + " |\n")
	}
	chunks := chunkMarkdown(b.String(), 400, 60)
	if len(chunks) < 2 {
		t.Fatalf("超长表格应被强制切分，实得 %d 块", len(chunks))
	}
	var withRows []string
	for i, c := range chunks {
		if !strings.Contains(c.Content, "ROW-") {
			continue
		}
		withRows = append(withRows, c.Content)
		// 每个表格碎片都必须携带表头+分隔行（首片是原生表头，续片是重发）
		if !strings.Contains(c.Content, "| 名称 | 说明 |") || !strings.Contains(c.Content, "| --- | --- |") {
			t.Errorf("表格碎片 %d 缺表头或分隔行", i)
		}
	}
	if len(withRows) < 2 {
		t.Fatalf("表格应被切成多片，实得 %d 片", len(withRows))
	}
	order := extractOrder(withRows, regexp.MustCompile(`ROW-(\d+)`))
	if len(order) != 30 {
		t.Fatalf("表格行应 30 行全保留，实得 %d 行", len(order))
	}
	assertStrictlyIncreasing(t, order, "表格行")
}

func TestChunkMarkdownMergesOrphanTail(t *testing.T) {
	var b strings.Builder
	b.WriteString("# 孤儿测试\n\n" + filler("BODY", 40) + "。\n\n尾巴。")
	chunks := chunkMarkdown(b.String(), 300, 40)
	for _, c := range chunks {
		if !strings.Contains(c.Content, "尾巴") {
			continue
		}
		// 「尾巴。」远小于最小块长，应并入前块而非单独成块（或被过滤丢失）
		if runeLen(c.Content) < 100 {
			t.Errorf("孤儿尾块未并入前块: %q", c.Content)
		}
		return
	}
	t.Fatal("找不到含尾块的块——尾部正文被丢弃了")
}

func TestChunkMarkdownDeterministic(t *testing.T) {
	doc := buildFixtureDoc()
	a := chunkMarkdown(doc, 300, 40)
	b := chunkMarkdown(doc, 300, 40)
	if !reflect.DeepEqual(a, b) {
		t.Error("两次切块结果不一致，切块器非确定性")
	}
}

func TestEmbedInputPrefix(t *testing.T) {
	c := IndexedChunk{Content: "正文", TitlePath: "调度原理 > 优先级"}
	if got, want := c.EmbedInput("React Fiber 深度解析"),
		"《React Fiber 深度解析》 > 调度原理 > 优先级\n\n正文"; got != want {
		t.Errorf("完整前缀:\n got %q\nwant %q", got, want)
	}
	if got, want := c.EmbedInput(""), "调度原理 > 优先级\n\n正文"; got != want {
		t.Errorf("无文章标题:\n got %q\nwant %q", got, want)
	}
	noPath := IndexedChunk{Content: "正文"}
	if got, want := noPath.EmbedInput("标题"), "《标题》\n\n正文"; got != want {
		t.Errorf("无章节路径:\n got %q\nwant %q", got, want)
	}
	if got := noPath.EmbedInput(""); got != "正文" {
		t.Errorf("无任何前缀应原样返回: %q", got)
	}
}

func TestChunkMarkdownEmptyAndTiny(t *testing.T) {
	for _, doc := range []string{"", "   \n\n  ", "## 只有标题", "# h1\n## h2\n### h3"} {
		chunks := chunkMarkdown(doc, 300, 40)
		if len(chunks) != 0 {
			t.Errorf("空/纯标题文档应切出零块，实得 %d 块: %v", len(chunks), chunks)
		}
	}
}

func TestCleanMarkdownForIndex(t *testing.T) {
	t.Run("单行注释", func(t *testing.T) {
		got := cleanMarkdownForIndex("前文\n<!-- 隐藏内容 -->\n后文")
		if strings.Contains(got, "隐藏") || strings.Contains(got, "<!--") {
			t.Errorf("注释未删除: %q", got)
		}
	})
	t.Run("跨行注释", func(t *testing.T) {
		got := cleanMarkdownForIndex("前文\n<!-- 第一行\n第二行 -->\n后文")
		if strings.Contains(got, "第一行") || strings.Contains(got, "第二行") {
			t.Errorf("跨行注释未删除: %q", got)
		}
		if !strings.Contains(got, "后文") {
			t.Errorf("误删注释后的正文: %q", got)
		}
	})
	t.Run("围栏内注释保留", func(t *testing.T) {
		got := cleanMarkdownForIndex("```html\n<!-- 保留我 -->\n```")
		if !strings.Contains(got, "<!-- 保留我 -->") {
			t.Errorf("围栏内注释被误删: %q", got)
		}
	})
	t.Run("figure占位符", func(t *testing.T) {
		got := cleanMarkdownForIndex("正文{{figure:fig-1}}尾部")
		if strings.Contains(got, "{{figure") {
			t.Errorf("占位符未删除: %q", got)
		}
	})
	t.Run("CRLF归一", func(t *testing.T) {
		got := cleanMarkdownForIndex("a\r\nb\r\nc")
		if strings.Contains(got, "\r") {
			t.Errorf("CRLF 未归一: %q", got)
		}
	})
	t.Run("空行压缩", func(t *testing.T) {
		got := cleanMarkdownForIndex("a\n\n\n\n\nb")
		if strings.Count(got, "\n\n") != 1 || strings.Contains(got, "\n\n\n") {
			t.Errorf("空行未压缩: %q", got)
		}
	})
}

package service

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// figurePlaceholderRe 匹配正文中 {{figure:xxx}} 配图占位符（向量检索不需要图片语法）。
var figurePlaceholderRe = regexp.MustCompile(`\{\{figure:[^}]*\}\}`)

// htmlCommentRe 匹配同一行内完整的 HTML 注释。
var htmlCommentRe = regexp.MustCompile(`<!--.*?-->`)

// headingRe 匹配 ATX 标题行（行首至多 3 空格 + 1~6 个 # + 空白/行尾）。
// 相比旧实现的无条件 HasPrefix("#")，代码块内的注释行不会再被误判为标题。
var headingRe = regexp.MustCompile(`^ {0,3}(#{1,6})(?:\s|$)`)

// listTopRe 匹配顶层列表条目行（无序 - * + 或有序 1. 1)）。
var listTopRe = regexp.MustCompile(`^ {0,3}(?:[-*+]|\d{1,3}[.)])(?:\s|$)`)

// tableDelimRe 匹配 GFM 表格分隔行（| --- | :---: | 这类，至少含一个 -）。
var tableDelimRe = regexp.MustCompile(`^ {0,3}\|?[\s:|-]*-[\s:|-]*\|?\s*$`)

// cleanMarkdownForIndex 清理不适合入库检索的标记：HTML 注释（围栏内保留）、
// 配图占位符、多余空行；并把 CRLF 归一为 LF（围栏/注释扫描都按 \n 行处理）。
func cleanMarkdownForIndex(md string) string {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	md = stripHTMLComments(md)
	md = figurePlaceholderRe.ReplaceAllString(md, "")
	// 连续空行压缩为单个，避免切块内容稀疏
	for strings.Contains(md, "\n\n\n") {
		md = strings.ReplaceAll(md, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(md)
}

// stripHTMLComments 删除围栏外的 HTML 注释（含跨行注释），围栏内的行原样保留。
// 整行都是注释的行直接丢弃，不留空洞。
func stripHTMLComments(md string) string {
	var out []string
	var fs fenceState
	inComment := false
	for _, raw := range strings.Split(md, "\n") {
		if fs.in { // 围栏内：不处理注释，原样保留
			out = append(out, raw)
			fenceAdvance(raw, &fs)
			continue
		}
		wasBlank := strings.TrimSpace(raw) == ""
		line := raw
		if inComment {
			idx := strings.Index(line, "-->")
			if idx < 0 {
				continue
			}
			inComment = false
			line = line[idx+3:]
		}
		for {
			loc := htmlCommentRe.FindStringIndex(line)
			if loc == nil {
				break
			}
			line = line[:loc[0]] + line[loc[1]:]
		}
		if idx := strings.Index(line, "<!--"); idx >= 0 {
			inComment = true
			line = line[:idx]
		}
		if !wasBlank && strings.TrimSpace(line) == "" {
			continue // 整行都是注释，丢弃
		}
		out = append(out, line)
		fenceAdvance(line, &fs)
	}
	return strings.Join(out, "\n")
}

// fenceState 代码围栏（``` 或 ~~~）扫描状态。
type fenceState struct {
	in     bool
	char   byte
	minLen int
	lang   string // 开栏行的 info string（如 ```python 的 python）
}

// fenceLine 判断一行是否为围栏标记行：行首至多 3 空格后跟 ≥3 个连续 ` 或 ~。
// 返回标记字符、连续数量与 info string。
func fenceLine(line string) (ch byte, n int, info string, ok bool) {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i > 3 || i >= len(line) {
		return 0, 0, "", false
	}
	c := line[i]
	if c != '`' && c != '~' {
		return 0, 0, "", false
	}
	j := i
	for j < len(line) && line[j] == c {
		j++
	}
	if j-i < 3 {
		return 0, 0, "", false
	}
	return c, j - i, strings.TrimSpace(line[j:]), true
}

// fenceAdvance 用一行推进围栏状态机：栏外遇标记行即开栏；栏内需同字符、
// 长度不小于开栏且其后仅空白的标记行才闭栏（CommonMark 规则）。
func fenceAdvance(line string, st *fenceState) {
	ch, n, info, ok := fenceLine(line)
	if !ok {
		return
	}
	if !st.in {
		st.in, st.char, st.minLen, st.lang = true, ch, n, info
		return
	}
	if ch == st.char && n >= st.minLen && info == "" {
		st.in = false
	}
}

// IndexedChunk 切块产物：块正文 + 所属标题路径。
type IndexedChunk struct {
	Content   string
	TitlePath string
}

// chunkPrefix 话题前缀：《文章标题》 > 章节路径。任一段为空则省略该段。
// H1 是文章标题不进 TitlePath（renderTitlePath 约定），由调用方单独传入，
// 使每个块都携带文章级主题锚点——不止开篇块（正文含 # 标题行）才有。
func chunkPrefix(articleTitle, sectionPath string) string {
	switch {
	case articleTitle != "" && sectionPath != "":
		return "《" + articleTitle + "》 > " + sectionPath
	case articleTitle != "":
		return "《" + articleTitle + "》"
	default:
		return sectionPath
	}
}

// EmbedInput 向量化输入：话题前缀（文章标题 + 标题路径）为块提供主题锚点，
// 纯代码块的 embedding 因此携带所在小节的语义。索引侧与重排侧共用此拼装
// （检索命中块经 IndexedChunk{...}.EmbedInput(hit.Title) 还原同样的输入）。
func (c IndexedChunk) EmbedInput(articleTitle string) string {
	if pre := chunkPrefix(articleTitle, c.TitlePath); pre != "" {
		return pre + "\n\n" + c.Content
	}
	return c.Content
}

// chunkMarkdown 把 markdown 切成带标题路径的向量块（中文按 rune 计长）：
// ① 按标题分节（围栏内 # 注释不算标题）② 相邻小节贪婪打包至接近 chunkSize
// ③ 超长节在结构边界硬切：代码围栏原子（超限碎片补回围栏行）、表格不拆行
//（超限碎片重发表头）、列表只在顶层条目边界断 ④ 孤儿尾块并入前块。
// 硬上限 hardCap 与最小块 minChunk 由 chunkSize 派生（算法正确性边界，
// 随 chunk_size 联动，不单独暴露配置）。
func chunkMarkdown(md string, chunkSize, overlap int) []IndexedChunk {
	if chunkSize <= 0 {
		chunkSize = 1000
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = chunkSize / 8
	}
	hardCap := chunkSize * 8 / 5
	minChunk := chunkSize / 5
	packed := packSections(splitSections(md), chunkSize)
	sized := make([]section, 0, len(packed))
	for _, s := range packed {
		if s.runes > chunkSize {
			sized = append(sized, hardSplitSection(s, chunkSize, overlap, hardCap)...)
		} else {
			sized = append(sized, s)
		}
	}
	return finalizeChunks(sized, minChunk, hardCap)
}

// runeLen 统一计长口径：按 rune 数（中文一个字符计 1）。
func runeLen(s string) int { return utf8.RuneCountInString(s) }

// section 语义节：同一标题路径下的连续行。
type section struct {
	titlePath string
	lines     []string
	runes     int // 含行尾换行的 rune 计长
}

type headingNode struct {
	level int
	text  string
}

// splitSections 按标题行把 markdown 切成语义节：标题行开新节并维护标题栈，
// 围栏内的行（含 # 注释）一律属于当前节，不触发分节。
func splitSections(md string) []section {
	var secs []section
	var stack []headingNode
	var fs fenceState
	cur := section{}
	flush := func() {
		if len(cur.lines) > 0 {
			secs = append(secs, cur)
		}
	}
	for _, line := range strings.Split(md, "\n") {
		if !fs.in && headingRe.MatchString(line) {
			flush()
			level, text := parseHeading(line)
			for len(stack) > 0 && stack[len(stack)-1].level >= level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, headingNode{level: level, text: text})
			cur = section{titlePath: renderTitlePath(stack)}
		}
		cur.lines = append(cur.lines, line)
		cur.runes += runeLen(line) + 1
		fenceAdvance(line, &fs)
	}
	flush()
	return secs
}

// parseHeading 解析 ATX 标题的级别与文本（去掉行首 #、尾部装饰 # 与空白）。
func parseHeading(line string) (int, string) {
	m := headingRe.FindStringSubmatch(line)
	if m == nil {
		return 0, ""
	}
	rest := strings.TrimLeft(line[strings.Index(line, "#"):], "#")
	rest = strings.TrimSpace(rest)
	rest = strings.TrimRight(rest, "#")
	return len(m[1]), strings.TrimSpace(rest)
}

// renderTitlePath 拼标题路径（" > " 连接），跳过 level-1——生成约定首个一级
// 标题即文章标题（llm/prompt.go 写作规则），不属于任何小节。
func renderTitlePath(stack []headingNode) string {
	parts := make([]string, 0, len(stack))
	for _, h := range stack {
		if h.level == 1 {
			continue
		}
		parts = append(parts, h.text)
	}
	return strings.Join(parts, " > ")
}

// packSections 相邻小节贪婪合并打包到接近 target 长度，减少碎块。
// 打包块的 titlePath 取合并中最后一个非空路径（更具体的小节锚点；
// 文首无标题的引言节因此会跟随首个正文小节的路径）。
func packSections(secs []section, target int) []section {
	var out []section
	var cur *section
	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
	}
	for _, s := range secs {
		if cur == nil {
			cp := s
			cur = &cp
			continue
		}
		if cur.runes+s.runes <= target {
			cur.lines = append(cur.lines, s.lines...)
			cur.runes += s.runes
			if s.titlePath != "" {
				cur.titlePath = s.titlePath
			}
		} else {
			flush()
			cp := s
			cur = &cp
		}
	}
	flush()
	return out
}

// hardSplitSection 把超长节在结构边界硬切成 ≤hardCap 的片段：
// 围栏优先等闭合（逼近 hardCap 才强制切并补回围栏行）；表格行不拆，
// 超限切点重发表头；列表只在顶层条目边界断；纯散文断点把上一片段尾部
// ≤overlap 的完整行拼进下一片段开头。结构边界断点不做 overlap（避免
// 条目/行重复）。
func hardSplitSection(sec section, target, overlap, hardCap int) []section {
	var pieces []section
	cur := section{titlePath: sec.titlePath}
	curRunes := 0
	var fs fenceState
	closeFence := "```"
	inTable := false
	var tableHeader []string // 表头行 + 分隔行（GFM 必须成对），跨片重发

	flush := func() {
		if curRunes > 0 {
			pieces = append(pieces, cur)
		}
		cur = section{titlePath: sec.titlePath}
		curRunes = 0
	}
	for _, line := range sec.lines {
		rl := runeLen(line) + 1

		// ── 围栏内：不受 target 约束，逼近 hardCap 才强制切
		if fs.in {
			if curRunes+rl > hardCap {
				cur.lines = append(cur.lines, closeFence)
				curRunes += runeLen(closeFence) + 1
				flush()
				cur.lines = append(cur.lines, closeFence+fs.lang) // 合成开栏，围栏态延续
				curRunes += runeLen(closeFence+fs.lang) + 1
			}
			cur.lines = append(cur.lines, line)
			curRunes += rl
			fenceAdvance(line, &fs)
			continue
		}

		// ── 表格行：不按 target 断；逼近 hardCap 才在行边界切并重发表头
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			switch {
			case !inTable:
				tableHeader = []string{line} // 表头候选，待下一行分隔行确认
			case len(tableHeader) == 1:
				if tableDelimRe.MatchString(strings.TrimSpace(line)) {
					tableHeader = append(tableHeader, line)
				} else {
					tableHeader = nil // 第二行不是分隔行，并非 GFM 表格
				}
			}
			if curRunes+rl > hardCap {
				flush()
				for _, h := range tableHeader {
					cur.lines = append(cur.lines, h)
					curRunes += runeLen(h) + 1
				}
			}
			cur.lines = append(cur.lines, line)
			curRunes += rl
			inTable = true
			fenceAdvance(line, &fs)
			continue
		}
		if inTable { // 表格到此结束
			inTable, tableHeader = false, nil
		}

		// ── 其余行：列表条目边界优先，散文次之，硬上限兜底
		isTop := listTopRe.MatchString(line)
		isIndented := strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")
		switch {
		case isTop && curRunes+rl > target:
			flush() // 条目边界，不做 overlap（避免条目重复）
		case isIndented && curRunes > 0 && curRunes+rl > hardCap:
			flush() // 兜底：单个条目/缩进块自身超硬上限，行边界强制断
		case !isTop && !isIndented && curRunes > 0 && curRunes+rl > target:
			prev := cur
			flush()
			for _, t := range tailCompleteLines(prev.lines, overlap) {
				cur.lines = append(cur.lines, t)
				curRunes += runeLen(t) + 1
			}
		}
		cur.lines = append(cur.lines, line)
		curRunes += rl
		fenceAdvance(line, &fs)
	}
	if fs.in && curRunes > 0 { // 围栏未闭合到节尾：补合成闭栏
		cur.lines = append(cur.lines, closeFence)
	}
	flush()
	return pieces
}

// tailCompleteLines 自尾部取完整行，累计 ≤maxRunes；遇围栏标记行或表格行即停
// （overlap 片段不得携带半个围栏或无表头的孤立行）。
func tailCompleteLines(lines []string, maxRunes int) []string {
	var rev []string
	total := 0
	for i := len(lines) - 1; i >= 0; i-- {
		if _, _, _, ok := fenceLine(lines[i]); ok {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			break
		}
		rl := runeLen(lines[i]) + 1
		if total+rl > maxRunes {
			break
		}
		rev = append(rev, lines[i])
		total += rl
	}
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	return rev
}

// finalizeChunks 收尾：孤儿尾块并入前一块（<minChunk 且放得下就并；修剪后
// 不足 30 字节的放不下也要并——丢正文结尾比块略超上限更伤）、过滤无信息
// 碎片（纯标题、空行），产出最终块。
func finalizeChunks(secs []section, minChunk, hardCap int) []IndexedChunk {
	if n := len(secs); n >= 2 {
		last := secs[n-1]
		prev := &secs[n-2]
		lastTrimmed := runeLen(strings.TrimSpace(strings.Join(last.lines, "\n")))
		fits := prev.runes+last.runes <= hardCap
		if lastTrimmed < 30 || (last.runes < minChunk && fits) {
			prev.lines = append(prev.lines, last.lines...)
			prev.runes += last.runes
			secs = secs[:n-1]
		}
	}
	out := make([]IndexedChunk, 0, len(secs))
	for _, s := range secs {
		content := strings.TrimSpace(strings.Join(s.lines, "\n"))
		if runeLen(content) < 30 {
			continue // 几乎无信息的碎片（纯标题、纯空行）
		}
		out = append(out, IndexedChunk{Content: content, TitlePath: s.titlePath})
	}
	return out
}

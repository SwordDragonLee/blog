package service

import (
	"regexp"
	"strings"
)

// figurePlaceholderRe 匹配正文中 {{figure:xxx}} 配图占位符（向量检索不需要图片语法）。
var figurePlaceholderRe = regexp.MustCompile(`\{\{figure:[^}]*\}\}`)

// cleanMarkdownForIndex 清理不适合入库检索的标记：配图占位符、HTML 注释、多余空行。
func cleanMarkdownForIndex(md string) string {
	md = figurePlaceholderRe.ReplaceAllString(md, "")
	md = strings.ReplaceAll(md, "\r\n", "\n")
	// 连续空行压缩为单个，避免切块内容稀疏
	for strings.Contains(md, "\n\n\n") {
		md = strings.ReplaceAll(md, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(md)
}

// chunkMarkdown 把 markdown 按标题边界与长度切块（中文按字符计）：
// 优先在标题处断开；单块超过 chunkSize 字符时强制切分，并把上一块尾部
// overlap 字符拼进下一块开头，避免答案句被截断在边界上。
func chunkMarkdown(md string, chunkSize, overlap int) []string {
	if chunkSize <= 0 {
		chunkSize = 800
	}
	if overlap < 0 || overlap >= chunkSize {
		overlap = chunkSize / 8
	}
	var chunks []string
	var cur []string
	curLen := 0

	flush := func() {
		if curLen > 0 {
			chunks = append(chunks, strings.TrimSpace(strings.Join(cur, "\n")))
		}
		cur = nil
		curLen = 0
	}

	for _, line := range linesOf(md) {
		trimmed := strings.TrimSpace(line)
		isHeading := strings.HasPrefix(trimmed, "#")
		// 标题是自然语义边界：当前块已有足够内容时先收尾，标题另起一块
		if isHeading && curLen >= chunkSize/2 {
			flush()
		}
		cur = append(cur, line)
		curLen += len([]rune(line))

		if curLen >= chunkSize {
			text := strings.Join(cur, "\n")
			chunks = append(chunks, strings.TrimSpace(text))
			r := []rune(text)
			start := len(r) - overlap
			if start < 0 {
				start = 0
			}
			cur = []string{string(r[start:])}
			curLen = overlap
		}
	}
	flush()

	// 过滤几乎无信息的碎片（纯标题、纯空行）
	out := chunks[:0]
	for _, c := range chunks {
		if len([]rune(strings.TrimSpace(c))) >= 30 {
			out = append(out, c)
		}
	}
	return out
}

// linesOf 按行拆分文本。
func linesOf(s string) []string {
	return strings.Split(s, "\n")
}

package svggen

import (
	"fmt"
	"strings"
)

// 统一暗色常量与画布宽度。
const (
	baseW    = 1200
	cBG      = "#0F172A"
	cBG2     = "#1E293B"
	cPanel   = "#16213A"
	cBorder  = "#2E3F5C"
	cPanel2  = "#1C2A47"
	cAccent  = "#38BDF8"
	cAccent2 = "#A78BFA"
	cAccent3 = "#34D399"
	cText    = "#E2E8F0"
	cMuted   = "#94A3B8"
)

// fontFamily SVG 文本字体。
const fontFamily = `'Segoe UI','PingFang SC','Microsoft YaHei',sans-serif`

// esc 转义 XML 特殊字符。
func esc(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

// textW 估算文本渲染宽度：CJK≈字号，拉丁≈0.62×字号。
func textW(s string, size float64) float64 {
	w := 0.0
	for _, r := range s {
		switch {
		case r >= 0x2E80:
			w += size
		case r == ' ':
			w += size * 0.30
		default:
			w += size * 0.62
		}
	}
	return w
}

// wrapText 按显示宽度断行（中文按字、拉丁按词）。
func wrapText(s string, maxW float64, size float64) []string {
	var lines []string
	cur := ""
	curW := 0.0
	flush := func() {
		lines = append(lines, cur)
		cur = ""
		curW = 0
	}
	var word string
	pushWord := func(word string) {
		ww := textW(word, size)
		spaceW := textW(" ", size)
		if cur == "" {
			cur, curW = word, ww
			return
		}
		if curW+spaceW+ww <= maxW {
			cur += " " + word
			curW += spaceW + ww
			return
		}
		flush()
		cur, curW = word, ww
	}
	for _, r := range s {
		if r >= 0x2E80 { // CJK 逐字断行
			cw := size
			if curW+cw > maxW {
				flush()
			}
			cur += string(r)
			curW += cw
			continue
		}
		if r == ' ' || r == '\n' {
			if word != "" {
				pushWord(word)
				word = ""
			}
			if r == '\n' {
				flush()
			}
			continue
		}
		word += string(r)
	}
	if word != "" {
		pushWord(word)
	}
	if cur != "" {
		flush()
	}
	return lines
}

// rectEl 圆角矩形元素。
func rectEl(x, y, w, h, rx float64, fill, stroke string) string {
	strokeAttr := ""
	if stroke != "" {
		strokeAttr = fmt.Sprintf(` stroke="%s"`, stroke)
	}
	return fmt.Sprintf(
		`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" fill="%s"%s/>`,
		x, y, w, h, rx, fill, strokeAttr)
}

// textEl 文本元素（anchor: start/middle/end）。
func textEl(x, y float64, s string, size float64, fill, weight, anchor string) string {
	return fmt.Sprintf(
		`<text x="%.1f" y="%.1f" font-size="%.0f" fill="%s" font-weight="%s" text-anchor="%s">%s</text>`,
		x, y, size, fill, weight, anchor, esc(s))
}

// lineEl 直线/连线元素。
func lineEl(x1, y1, x2, y2 float64, stroke string, width float64, dashed bool) string {
	dash := ""
	if dashed {
		dash = ` stroke-dasharray="6 5"`
	}
	return fmt.Sprintf(
		`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f"%s/>`,
		x1, y1, x2, y2, stroke, width, dash)
}

// svgOpen 生成 SVG 头部（含渐变与箭头定义）。
func svgOpen(w, h int) string {
	return fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="%s">`,
		w, h, w, h, fontFamily) +
		fmt.Sprintf(
			`<defs>
<linearGradient id="bgGrad" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="%s"/><stop offset="1" stop-color="%s"/>
</linearGradient>
<marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse">
<path d="M0,0 L10,5 L0,10 z" fill="%s"/></marker>
</defs>`,
			cBG, cBG2, cMuted) +
		fmt.Sprintf(`<rect width="%d" height="%d" fill="url(#bgGrad)"/>`, w, h)
}

// svgClose 闭合 svg 标签。
func svgClose() string { return `</svg>` }

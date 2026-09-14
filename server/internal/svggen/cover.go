package svggen

import (
	"fmt"
	"strings"
)

// CoverInput 封面渲染输入。
type CoverInput struct {
	Title    string   `json:"title"`
	Tags     []string `json:"tags"`
	Subtitle string   `json:"subtitle"`
}

// RenderCover 渲染 1200x630 封面卡片：渐变背景 + 细网格 + 品牌条 + 标题/副标题/标签。
func RenderCover(in CoverInput) string {
	var b strings.Builder
	b.WriteString(svgOpen(1200, 630))

	// 细网格装饰
	for x := 0; x <= 1200; x += 60 {
		b.WriteString(fmt.Sprintf(`<line x1="%d" y1="0" x2="%d" y2="630" stroke="%s" stroke-opacity="0.05"/>`, x, x, cMuted))
	}
	for y := 0; y <= 630; y += 60 {
		b.WriteString(fmt.Sprintf(`<line x1="0" y1="%d" x2="1200" y2="%d" stroke="%s" stroke-opacity="0.05"/>`, y, y, cMuted))
	}

	// 顶部品牌条
	b.WriteString(rectEl(0, 0, 1200, 6, 0, cAccent, ""))

	// 右上角「AI 生成」角标
	badge := "AI 生成"
	bw := textW(badge, 16) + 56 // 含左侧圆点与两侧留白
	bx := 1200 - 60 - bw
	b.WriteString(rectEl(bx, 34, bw, 34, 17, cPanel2, cBorder))
	b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="51" r="4" fill="%s"/>`, bx+20, cAccent3))
	b.WriteString(textEl(bx+32, 57, badge, 16, cMuted, "600", "start"))

	// 标题上方强调短线
	b.WriteString(rectEl(60, 186, 64, 6, 3, cAccent, ""))

	// 主标题：最多 3 行
	lines := wrapText(in.Title, 780, 54)
	if len(lines) == 0 {
		lines = []string{"未命名文章"}
	}
	if len(lines) > 3 {
		lines = lines[:3]
		lines[2], _ = fitText(lines[2]+"…", 780, 54)
	}
	for i, ln := range lines {
		b.WriteString(textEl(60, 266+float64(i)*70, ln, 54, cText, "700", "start"))
	}

	// 副标题
	if sub := strings.TrimSpace(in.Subtitle); sub != "" {
		subBase := 266 + float64(len(lines)-1)*70 + 58
		s, sz := fitText(sub, 1080, 22)
		b.WriteString(textEl(60, subBase, s, sz, cMuted, "400", "start"))
	}

	// 底部标签 chips
	chipY := 524.0
	cx := 60.0
	for _, t := range in.Tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		label, _ := fitText(t, 1000, 20)
		w := textW(label, 20) + 36
		if cx+w > 1140 {
			break
		}
		b.WriteString(rectEl(cx, chipY, w, 40, 20, cPanel, cBorder))
		b.WriteString(textEl(cx+w/2, chipY+26, label, 20, cMuted, "500", "middle"))
		cx += w + 14
	}

	b.WriteString(svgClose())
	return b.String()
}

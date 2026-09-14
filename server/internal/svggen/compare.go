package svggen

import (
	"encoding/json"
	"fmt"
	"strings"
)

// compareInput compare 图数据：左右两侧对比。
type compareInput struct {
	Left  compareSide `json:"left"`
	Right compareSide `json:"right"`
}

type compareSide struct {
	Title  string   `json:"title"`
	Points []string `json:"points"`
}

const (
	cmpPanelW = 526.0 // 单侧面板宽
	cmpGap    = 28.0  // 中缝间距
)

// renderCompare 渲染对比图：左右双面板 + 要点列表 + 中缝 VS 徽标。
func renderCompare(title string, data string) (string, error) {
	var in compareInput
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		return "", fmt.Errorf("compare 数据解析失败: %w", err)
	}

	rows := len(in.Left.Points)
	if len(in.Right.Points) > rows {
		rows = len(in.Right.Points)
	}
	if rows < 3 {
		rows = 3 // 保证两侧等高
	}
	empty := in.Left.Title == "" && in.Right.Title == "" &&
		len(in.Left.Points) == 0 && len(in.Right.Points) == 0

	h := 152 + 84 + rows*44 + 12 + 56
	if empty {
		h = 480
	}

	var b strings.Builder
	b.WriteString(svgOpen(baseW, h))
	top := drawHeader(&b, title)

	if empty {
		emptyState(&b, top+140)
		b.WriteString(svgClose())
		return b.String(), nil
	}

	panelH := 84 + float64(rows)*44 + 12
	drawSide := func(px float64, side compareSide, accent, tag string) {
		b.WriteString(rectEl(px, top, cmpPanelW, panelH, 16, cPanel, cBorder))
		b.WriteString(rectEl(px+24, top+26, 4, 24, 2, accent, ""))
		t, sz := fitText(strings.TrimSpace(side.Title), cmpPanelW-140, 20)
		b.WriteString(textEl(px+40, top+45, t, sz, cText, "700", "start"))

		// 右上角 A/B 徽标
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="15" fill="%s" stroke="%s" stroke-width="1.5"/>`,
			px+cmpPanelW-38, top+38, cPanel2, accent))
		b.WriteString(textEl(px+cmpPanelW-38, top+43, tag, 14, accent, "700", "middle"))

		// 头部分隔线
		b.WriteString(lineEl(px+24, top+72, px+cmpPanelW-24, top+72, cBorder, 1, false))

		// 要点列表
		for i, p := range side.Points {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			py := top + 112 + float64(i)*44
			b.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="4" fill="%s"/>`, px+38, py-5, accent))
			s, ssz := fitText(p, cmpPanelW-96, 16)
			b.WriteString(textEl(px+56, py, s, ssz, cText, "400", "start"))
		}
	}

	drawSide(60, in.Left, cAccent, "A")
	drawSide(60+cmpPanelW+cmpGap, in.Right, cAccent2, "B")

	// 中缝 VS 徽标（横跨两面板）
	b.WriteString(rectEl(572, top+22, 56, 32, 16, cPanel2, cBorder))
	b.WriteString(textEl(600, top+43, "VS", 15, cMuted, "700", "middle"))

	b.WriteString(svgClose())
	return b.String(), nil
}

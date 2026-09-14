package svggen

import (
	"encoding/json"
	"fmt"
	"strings"
)

// flowInput flow 图数据：顺序步骤。
type flowInput struct {
	Steps []flowStep `json:"steps"`
}

type flowStep struct {
	Name string `json:"name"`
	Desc string `json:"desc"`
}

const (
	flowRowH   = 92.0  // 每步行高
	flowCardH  = 72.0  // 步骤卡片高度
	flowSpineX = 118.0 // 序号脊线 x
)

// renderFlow 渲染流程图：纵向步骤卡片 + 序号圆徽 + 虚线脊线。
func renderFlow(title string, data string) (string, error) {
	var in flowInput
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		return "", fmt.Errorf("flow 数据解析失败: %w", err)
	}

	h := 480
	if n := len(in.Steps); n > 0 {
		h = 152 + n*int(flowRowH) + 44
	}

	var b strings.Builder
	b.WriteString(svgOpen(baseW, h))
	top := drawHeader(&b, title)

	if len(in.Steps) == 0 {
		emptyState(&b, top+140)
		b.WriteString(svgClose())
		return b.String(), nil
	}

	firstC := top + flowCardH/2
	lastC := top + float64(len(in.Steps)-1)*flowRowH + flowCardH/2

	// 脊线（虚线）与末端箭头
	b.WriteString(lineEl(flowSpineX, firstC, flowSpineX, lastC, cBorder, 2, true))
	b.WriteString(arrowEl(flowSpineX, lastC, flowSpineX, lastC+30, cMuted, 2))

	for i, st := range in.Steps {
		cardY := top + float64(i)*flowRowH
		cy := cardY + flowCardH/2
		accent := figAccent(i)

		// 序号圆徽
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="21" fill="%s" stroke="%s" stroke-width="2"/>`,
			flowSpineX, cy, cPanel2, accent))
		b.WriteString(textEl(flowSpineX, cy+6, fmt.Sprintf("%d", i+1), 18, accent, "700", "middle"))

		// 步骤卡片
		b.WriteString(rectEl(170, cardY, 970, flowCardH, 12, cPanel, cBorder))
		b.WriteString(rectEl(188, cardY+18, 4, 36, 2, accent, ""))
		name, nsz := fitText(st.Name, 900, 19)
		b.WriteString(textEl(208, cardY+31, name, nsz, cText, "700", "start"))
		if d := strings.TrimSpace(st.Desc); d != "" {
			desc, dsz := fitText(d, 900, 15)
			b.WriteString(textEl(208, cardY+55, desc, dsz, cMuted, "400", "start"))
		}
	}

	b.WriteString(svgClose())
	return b.String(), nil
}

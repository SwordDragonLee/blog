package svggen

import (
	"encoding/json"
	"fmt"
	"strings"
)

// timelineInput timeline 图数据：时间线事件。
type timelineInput struct {
	Items []timelineItem `json:"items"`
}

type timelineItem struct {
	Time  string `json:"time"`
	Title string `json:"title"`
	Desc  string `json:"desc"`
}

const (
	tlRowH   = 100.0 // 每项行高
	tlCardH  = 76.0  // 事件卡片高度
	tlSpineX = 244.0 // 时间轴脊线 x
)

// renderTimeline 渲染时间线：左列时间 + 双环节点 + 右侧事件卡片。
func renderTimeline(title string, data string) (string, error) {
	var in timelineInput
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		return "", fmt.Errorf("timeline 数据解析失败: %w", err)
	}

	h := 480
	if n := len(in.Items); n > 0 {
		h = 152 + n*int(tlRowH) + 40
	}

	var b strings.Builder
	b.WriteString(svgOpen(baseW, h))
	top := drawHeader(&b, title)

	if len(in.Items) == 0 {
		emptyState(&b, top+140)
		b.WriteString(svgClose())
		return b.String(), nil
	}

	firstC := top + tlCardH/2
	lastC := top + float64(len(in.Items)-1)*tlRowH + tlCardH/2

	// 脊线（虚线）与末端箭头
	b.WriteString(lineEl(tlSpineX, firstC, tlSpineX, lastC, cBorder, 2, true))
	b.WriteString(arrowEl(tlSpineX, lastC, tlSpineX, lastC+30, cMuted, 2))

	for i, it := range in.Items {
		cardY := top + float64(i)*tlRowH
		cy := cardY + tlCardH/2
		accent := figAccent(i)

		// 时间列（右对齐）
		if tm := strings.TrimSpace(it.Time); tm != "" {
			s, sz := fitText(tm, 150, 16)
			b.WriteString(textEl(212, cy+6, s, sz, cMuted, "600", "end"))
		}

		// 节点圆点（双环）
		b.WriteString(fmt.Sprintf(
			`<circle cx="%.1f" cy="%.1f" r="9" fill="%s" stroke="%s" stroke-width="2.5"/>`+
				`<circle cx="%.1f" cy="%.1f" r="3.5" fill="%s"/>`,
			tlSpineX, cy, cBG, accent, tlSpineX, cy, accent))

		// 事件卡片
		b.WriteString(rectEl(288, cardY, 852, tlCardH, 12, cPanel, cBorder))
		b.WriteString(rectEl(306, cardY+20, 4, 36, 2, accent, ""))
		tt, tsz := fitText(strings.TrimSpace(it.Title), 780, 18)
		b.WriteString(textEl(326, cardY+31, tt, tsz, cText, "700", "start"))
		if d := strings.TrimSpace(it.Desc); d != "" {
			desc, dsz := fitText(d, 780, 14)
			b.WriteString(textEl(326, cardY+56, desc, dsz, cMuted, "400", "start"))
		}
	}

	b.WriteString(svgClose())
	return b.String(), nil
}

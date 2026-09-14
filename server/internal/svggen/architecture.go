package svggen

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ---- 包内各图表共用的绘制小工具（flow/compare/timeline 复用） ----

// drawHeader 绘制图表标题区（强调短线 + 标题 + 分隔线），返回内容区起始 y。
func drawHeader(b *strings.Builder, title string) float64 {
	b.WriteString(rectEl(60, 46, 46, 6, 3, cAccent, ""))
	if strings.TrimSpace(title) == "" {
		title = "未命名图表"
	}
	b.WriteString(textEl(60, 100, title, 30, cText, "700", "start"))
	b.WriteString(lineEl(60, 124, 1140, 124, cBorder, 1, false))
	return 152
}

// figAccent 第 i 个循环强调色。
func figAccent(i int) string {
	switch i % 3 {
	case 0:
		return cAccent
	case 1:
		return cAccent2
	default:
		return cAccent3
	}
}

// fitText 缩小一档字号或按字符截断加省略号，使文本适配 maxW；返回适配后的文本与字号。
func fitText(s string, maxW float64, size float64) (string, float64) {
	if maxW <= 0 {
		return "", size
	}
	if textW(s, size) <= maxW {
		return s, size
	}
	if size-3 >= 12 && textW(s, size-3) <= maxW {
		return s, size - 3
	}
	rs := []rune(s)
	for n := len(rs); n > 0; n-- {
		cand := string(rs[:n]) + "…"
		if textW(cand, size) <= maxW {
			return cand, size
		}
	}
	return "…", size
}

// arrowEl 带箭头连线（复用 svgOpen 预定义的 arrow marker）。
func arrowEl(x1, y1, x2, y2 float64, stroke string, width float64) string {
	return fmt.Sprintf(
		`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="%.1f" marker-end="url(#arrow)"/>`,
		x1, y1, x2, y2, stroke, width)
}

// emptyState 数据为空时的占位提示。
func emptyState(b *strings.Builder, cy float64) {
	b.WriteString(rectEl(60, cy-40, 1080, 80, 14, cPanel, cBorder))
	b.WriteString(textEl(600, cy+7, "暂无数据", 18, cMuted, "400", "middle"))
}

// ---- architecture 分层架构图 ----

// archInput architecture 图数据：分层与跨层连线。
type archInput struct {
	Layers []archLayer `json:"layers"`
	Edges  []archEdge  `json:"edges"`
}

type archLayer struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

type archEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

const (
	archBandH   = 96.0  // 每层横带高度
	archBandGap = 56.0  // 层间距（走线区）
	archNodeH   = 52.0  // 节点高度
	archNodeX0  = 210.0 // 节点区起点 x
	archNodeW   = 930.0 // 节点区总宽
)

// renderArchitecture 渲染分层架构图：横向层带 + 层内节点 + 跨层连线。
func renderArchitecture(title string, data string) (string, error) {
	var in archInput
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		return "", fmt.Errorf("architecture 数据解析失败: %w", err)
	}

	h := 480
	if n := len(in.Layers); n > 0 {
		h = 152 + n*int(archBandH) + (n-1)*int(archBandGap) + 52
		if h < 480 {
			h = 480
		}
	}

	var b strings.Builder
	b.WriteString(svgOpen(baseW, h))
	top := drawHeader(&b, title)

	if len(in.Layers) == 0 {
		emptyState(&b, top+140)
		b.WriteString(svgClose())
		return b.String(), nil
	}

	// 各层 y 与节点中心定位（重名节点取首次出现）
	type nodeBox struct {
		cx, cy float64
		band   int
	}
	boxes := map[string]nodeBox{}
	bandY := make([]float64, len(in.Layers))
	for li := range in.Layers {
		bandY[li] = top + float64(li)*(archBandH+archBandGap)
	}
	for li, layer := range in.Layers {
		cnt := len(layer.Nodes)
		if cnt == 0 {
			continue
		}
		gap := 14.0
		nodeW := (archNodeW - float64(cnt-1)*gap) / float64(cnt)
		for ni, name := range layer.Nodes {
			if name == "" {
				continue
			}
			if _, dup := boxes[name]; dup {
				continue
			}
			boxes[name] = nodeBox{
				cx:   archNodeX0 + float64(ni)*(nodeW+gap) + nodeW/2,
				cy:   bandY[li] + archBandH/2,
				band: li,
			}
		}
	}

	// 连线：相邻层连线画在面板上层，跨层连线画在面板下层（中段被面板自然遮盖）
	drawEdge := func(e archEdge, under bool) {
		f, okF := boxes[e.From]
		t, okT := boxes[e.To]
		if !okF || !okT || f.band == t.band {
			return
		}
		adjacent := t.band-f.band == 1 || f.band-t.band == 1
		if under == adjacent {
			return
		}
		var x1, y1, x2, y2 float64
		if t.band > f.band {
			x1, y1 = f.cx, f.cy+archNodeH/2
			x2 = t.cx
			if adjacent {
				y2 = t.cy - archNodeH/2
			} else {
				y2 = bandY[t.band] - 3
			}
		} else {
			x1, y1 = f.cx, f.cy-archNodeH/2
			x2 = t.cx
			if adjacent {
				y2 = t.cy + archNodeH/2
			} else {
				y2 = bandY[t.band] + archBandH + 3
			}
		}
		b.WriteString(arrowEl(x1, y1, x2, y2, cMuted, 1.5))
		if label := strings.TrimSpace(e.Label); !under && label != "" {
			my := (y1+y2)/2 + 4
			lw := textW(label, 13)
			if x2-x1 < 60 && x1-x2 < 60 { // 近垂直连线：标签放右侧避让
				lx := max(x1, x2) + 12
				if lx+lw > 1140 {
					lx = 1140 - lw
				}
				b.WriteString(textEl(lx, my, label, 13, cMuted, "400", "start"))
			} else {
				mx := min(max((x1+x2)/2, 60+lw/2), 1140-lw/2)
				b.WriteString(textEl(mx, my, label, 13, cMuted, "400", "middle"))
			}
		}
	}
	for _, e := range in.Edges {
		drawEdge(e, true)
	}

	// 层带 + 层名 + 节点
	for li, layer := range in.Layers {
		y := bandY[li]
		accent := figAccent(li)
		b.WriteString(rectEl(60, y, 1080, archBandH, 14, cPanel, cBorder))
		b.WriteString(rectEl(78, y+archBandH/2-16, 4, 32, 2, accent, ""))
		if name := strings.TrimSpace(layer.Name); name != "" {
			s, sz := fitText(name, 106, 17)
			b.WriteString(textEl(94, y+archBandH/2+6, s, sz, accent, "700", "start"))
		}
		cnt := len(layer.Nodes)
		if cnt == 0 {
			continue
		}
		gap := 14.0
		nodeW := (archNodeW - float64(cnt-1)*gap) / float64(cnt)
		for ni, node := range layer.Nodes {
			if node == "" {
				continue
			}
			nx := archNodeX0 + float64(ni)*(nodeW+gap)
			ny := y + (archBandH-archNodeH)/2
			b.WriteString(rectEl(nx, ny, nodeW, archNodeH, 10, cPanel2, cBorder))
			s, sz := fitText(node, nodeW-20, 17)
			b.WriteString(textEl(nx+nodeW/2, ny+archNodeH/2+6, s, sz, cText, "600", "middle"))
		}
	}

	// 相邻层连线（带标签）绘制在面板之上
	for _, e := range in.Edges {
		drawEdge(e, false)
	}

	b.WriteString(svgClose())
	return b.String(), nil
}

// Package svggen 将结构化图表数据确定性渲染为风格统一的 SVG 文档。
package svggen

import (
	"fmt"

	"blog/server/internal/model"
)

type renderFn func(title string, data string) (string, error)

var renderers = map[string]renderFn{
	model.FigureKindArchitecture: renderArchitecture,
	model.FigureKindFlow:         renderFlow,
	model.FigureKindCompare:      renderCompare,
	model.FigureKindTimeline:     renderTimeline,
}

// Render 按图类型分发渲染，data 为 LLM 配图数据（JSON 字符串）。
func Render(kind, title string, data string) (string, error) {
	fn := renderers[kind]
	if fn == nil {
		return "", fmt.Errorf("不支持的图类型: %s", kind)
	}
	return fn(title, data)
}

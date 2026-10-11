package model

import (
	"time"
)

// FigureKind 配图类型：cover 封面 + 四种业务图表。
type FigureKind string

const (
	FigureKindCover        FigureKind = "cover"
	FigureKindArchitecture FigureKind = "architecture"
	FigureKindFlow         FigureKind = "flow"
	FigureKindCompare      FigureKind = "compare"
	FigureKindTimeline     FigureKind = "timeline"
)

// SvgAsset 与文章关联的 SVG 配图；正文中以 {{figure:placeholder}} 占位引用。
type SvgAsset struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	ArticleID   uint           `gorm:"index" json:"article_id"`
	Placeholder string         `gorm:"size:32;index" json:"placeholder"` // 正文中的 {{figure:xxx}} 标识
	Kind        FigureKind     `gorm:"size:32" json:"kind"`
	Title       string         `gorm:"size:256" json:"title"`
	Spec        JSON           `json:"spec" swaggertype:"object"`
	SVGContent  string         `gorm:"mediumtext" json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (SvgAsset) TableName() string { return "svg_asset" }

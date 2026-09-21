package model

import (
	"time"

	"gorm.io/datatypes"
)

// 配图类型：cover 封面 + 四种业务图表。
const (
	FigureKindCover        = "cover"
	FigureKindArchitecture = "architecture"
	FigureKindFlow         = "flow"
	FigureKindCompare      = "compare"
	FigureKindTimeline     = "timeline"
)

// SvgAsset 与文章关联的 SVG 配图；正文中以 {{figure:placeholder}} 占位引用。
type SvgAsset struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	ArticleID   uint           `gorm:"index" json:"article_id"`
	Placeholder string         `gorm:"size:32;index" json:"placeholder"` // 正文中的 {{figure:xxx}} 标识
	Kind        string         `gorm:"size:32" json:"kind"`
	Title       string         `gorm:"size:256" json:"title"`
	Spec        datatypes.JSON `json:"spec"`
	SVGContent  string         `gorm:"mediumtext" json:"-"`
	CreatedAt   time.Time      `json:"created_at"`
}

func (SvgAsset) TableName() string { return "svg_asset" }

package llm

import "encoding/json"

// Message 是 OpenAI 兼容接口的对话消息。
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []Message       `json:"messages"`
	Temperature    float32         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// anthropicResponse 是 Anthropic Messages 协议的响应体，
// content 为分块数组，仅拼接 type=text 的文本块（忽略 thinking 等块）。
type anthropicResponse struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// FigureOut 是 LLM 写作时输出的单张配图描述。
type FigureOut struct {
	ID    string          `json:"id"`   // 正文占位符标识，如 fig-1
	Kind  string          `json:"kind"` // architecture / flow / compare / timeline
	Title string          `json:"title"`
	Data  json.RawMessage `json:"data"` // 具体结构由 svggen 按 kind 解析
}

// ArticleOut 是 LLM 单篇写作的结构化输出。
type ArticleOut struct {
	Title    string      `json:"title"`
	Slug     string      `json:"slug"`
	Summary  string      `json:"summary"`
	Tags     []string    `json:"tags"`
	Markdown string      `json:"markdown"`
	Figures  []FigureOut `json:"figures"`
}

// ArticlePlanOut 是仓库分析输出中的单篇文章选题。
type ArticlePlanOut struct {
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	Summary     string   `json:"summary"`
	Tags        []string `json:"tags"`
	Outline     []string `json:"outline"`
	TargetWords int      `json:"target_words"`
}

// AnalysisOut 是仓库分析阶段的结构化输出。
type AnalysisOut struct {
	RepoName    string           `json:"repo_name"`
	TechStack   []string         `json:"tech_stack"`
	Summary     string           `json:"summary"`
	Highlights  []string         `json:"highlights"`
	ArticlePlan []ArticlePlanOut `json:"article_plan"`
}

// FiguresOut 是配图重生成阶段的结构化输出。
type FiguresOut struct {
	Figures []FigureOut `json:"figures"`
}

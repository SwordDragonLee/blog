// RAG 索引与问答运营（与后端 service 层响应结构对齐，JSON tag 同名）

// RAG 向量索引总览（按文章分组的向量块统计）
export interface RagIndexStat {
  slug: string;
  title: string;
  chunks: number;
}

export interface RagIndexOverview {
  articles: RagIndexStat[];
  total_chunks: number;
}

// 检索测试命中（pass = 得分达到阈值，真实问答会采纳）
export interface RagProbeHit {
  score: number;
  slug: string;
  title: string;
  snippet: string;
  pass: boolean;
}

// 问答无命中问题：选题回流（同一归一化键聚合，hits 为提问次数）
export interface AskUnanswered {
  id: number;
  question: string;
  normalized_key: string;
  hits: number;
  first_seen_at: string;
  last_seen_at: string;
}

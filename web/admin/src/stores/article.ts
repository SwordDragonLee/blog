import { makeAutoObservable, runInAction } from 'mobx';
import { get, post, put } from '../api/http';
import type { Article, ArticleDetail, ArticleStatus, ArticleUpdatePayload, PageData } from '../types';

export class ArticleStore {
  items: Article[] = [];
  total = 0;
  page = 1;
  pageSize = 10;
  status: ArticleStatus | '' = '';
  loading = false;

  current: ArticleDetail | null = null;
  detailLoading = false;
  saving = false;
  publishing = false;
  regenerating = false;

  constructor() {
    makeAutoObservable(this);
  }

  async fetchArticles(
    status: ArticleStatus | '' = this.status,
    page = this.page,
    pageSize = this.pageSize,
  ): Promise<void> {
    this.loading = true;
    try {
      const params: Record<string, unknown> = { page, page_size: pageSize };
      if (status) params.status = status;
      const data = await get<PageData<Article>>('/articles', params);
      runInAction(() => {
        this.items = data.items || [];
        this.total = data.total ?? this.items.length;
        this.page = data.page ?? page;
        this.pageSize = data.page_size ?? pageSize;
        this.status = status;
      });
    } finally {
      runInAction(() => {
        this.loading = false;
      });
    }
  }

  async fetchArticle(id: number): Promise<ArticleDetail> {
    this.detailLoading = true;
    try {
      const article = await get<ArticleDetail>(`/articles/${id}`);
      runInAction(() => {
        this.current = { ...article, figures: article.figures || [] };
      });
      return article;
    } finally {
      runInAction(() => {
        this.detailLoading = false;
      });
    }
  }

  async save(id: number, payload: ArticleUpdatePayload): Promise<Article> {
    this.saving = true;
    try {
      const article = await put<Article>(`/articles/${id}`, payload);
      runInAction(() => {
        if (this.current && this.current.id === id) {
          this.current = { ...this.current, ...article, figures: this.current.figures };
        }
      });
      await this.fetchArticles();
      return article;
    } finally {
      runInAction(() => {
        this.saving = false;
      });
    }
  }

  /** 发版：draft -> published */
  async publish(id: number): Promise<void> {
    this.publishing = true;
    try {
      await post(`/articles/${id}/publish`);
      if (this.current?.id === id) await this.fetchArticle(id);
      await this.fetchArticles();
    } finally {
      runInAction(() => {
        this.publishing = false;
      });
    }
  }

  /** 下线：published -> draft */
  async offline(id: number): Promise<void> {
    await post(`/articles/${id}/offline`);
    if (this.current?.id === id) await this.fetchArticle(id);
    await this.fetchArticles();
  }

  /** 重新生成该篇配图，完成后刷新详情（含新配图） */
  async regenerateFigures(id: number): Promise<void> {
    this.regenerating = true;
    try {
      // 配图重跑涉及 LLM 调用，放宽超时
      await post(`/articles/${id}/regenerate-figures`, undefined, { timeout: 300000 });
      if (this.current?.id === id) await this.fetchArticle(id);
    } finally {
      runInAction(() => {
        this.regenerating = false;
      });
    }
  }
}

export const articleStore = new ArticleStore();

import { makeAutoObservable, runInAction } from 'mobx';
import dayjs from 'dayjs';
import { get, post, del } from '../api/http';
import type { GenTask, PageData, TaskStatus } from '../types';

const POLL_INTERVAL = 2000;

export class TaskStore {
  items: GenTask[] = [];
  total = 0;
  page = 1;
  pageSize = 10;
  loading = false;

  current: GenTask | null = null;
  currentLoading = false;
  logs: string[] = [];

  private pollTimer: number | null = null;

  constructor() {
    makeAutoObservable(this);
  }

  async fetchTasks(page = this.page, pageSize = this.pageSize): Promise<void> {
    this.loading = true;
    try {
      const data = await get<PageData<GenTask>>('/tasks', { page, page_size: pageSize });
      runInAction(() => {
        this.items = data.items || [];
        this.total = data.total ?? this.items.length;
        this.page = data.page ?? page;
        this.pageSize = data.page_size ?? pageSize;
      });
    } finally {
      runInAction(() => {
        this.loading = false;
      });
    }
  }

  async createTask(gitUrl: string): Promise<GenTask> {
    const task = await post<GenTask>('/tasks', { git_url: gitUrl });
    await this.fetchTasks(1);
    return task;
  }

  async retry(id: number): Promise<void> {
    await post(`/tasks/${id}/retry`);
    await this.fetchTask(id);
    this.startPolling(id);
  }

  /** 取消排队中/运行中的任务 */
  async cancel(id: number): Promise<void> {
    await post(`/tasks/${id}/cancel`);
    this.stopPolling();
    await this.fetchTask(id).catch(() => undefined);
    await this.fetchTasks();
  }

  /** 删除任务及其衍生数据（草稿文章、配图、分析记录） */
  async remove(id: number): Promise<void> {
    await del(`/tasks/${id}`);
    if (this.current?.id === id) this.resetCurrent();
    await this.fetchTasks();
  }

  async fetchTask(id: number): Promise<GenTask> {
    this.currentLoading = true;
    try {
      const task = await get<GenTask>(`/tasks/${id}`);
      runInAction(() => {
        this.current = task;
        this.appendLog(task);
      });
      return task;
    } finally {
      runInAction(() => {
        this.currentLoading = false;
      });
    }
  }

  /** 每 2s 轮询当前任务，直到 success/failed */
  startPolling(id: number): void {
    this.stopPolling();
    this.pollTimer = window.setInterval(() => {
      void (async () => {
        try {
          const task = await get<GenTask>(`/tasks/${id}`);
          runInAction(() => {
            this.current = task;
            this.appendLog(task);
          });
          if (
            task.status === 'success' ||
            task.status === 'failed' ||
            task.status === 'canceled'
          ) {
            this.stopPolling();
          }
        } catch {
          // 轮询失败静默重试，不打断轮询节奏
        }
      })();
    }, POLL_INTERVAL);
  }

  stopPolling(): void {
    if (this.pollTimer !== null) {
      window.clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  get isCurrentRunning(): boolean {
    const s: TaskStatus | undefined = this.current?.status;
    return s === 'pending' || s === 'running';
  }

  /** step/message 有变化时追加一条日志 */
  private appendLog(task: GenTask): void {
    if (!task.message) return;
    const body = `${task.step || task.status} - ${task.message}`;
    const last = this.logs[this.logs.length - 1];
    if (last && last.endsWith(`] ${body}`)) return;
    this.logs.push(`[${dayjs().format('HH:mm:ss')}] ${body}`);
    if (this.logs.length > 500) this.logs.splice(0, this.logs.length - 500);
  }

  resetCurrent(): void {
    this.stopPolling();
    this.current = null;
    this.logs = [];
  }
}

export const taskStore = new TaskStore();

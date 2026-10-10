import { makeAutoObservable, runInAction } from 'mobx';
import { post } from '../api/http';
import { TOKEN_KEY } from '../api/http';
import type { AdminUser, LoginResp } from '../types';

const USER_KEY = 'blog_admin_user';

export class AuthStore {
  token: string | null = localStorage.getItem(TOKEN_KEY);
  user: AdminUser | null = null;
  loggingIn = false;

  constructor() {
    makeAutoObservable(this);
    const raw = localStorage.getItem(USER_KEY);
    if (raw) {
      try {
        this.user = JSON.parse(raw) as AdminUser;
      } catch {
        this.user = null;
      }
    }
  }

  get isAuthenticated(): boolean {
    return !!this.token;
  }

  get username(): string {
    return this.user?.username || 'admin';
  }

  async login(username: string, password: string): Promise<void> {
    this.loggingIn = true;
    try {
      const data = await post<LoginResp>('/auth/login', { username, password });
      runInAction(() => {
        this.setSession(data.token, data.user || { id: 0, username });
      });
    } finally {
      runInAction(() => {
        this.loggingIn = false;
      });
    }
  }

  /** 全量替换会话：token + user 同步写内存与 localStorage（登录、改用户名后换发） */
  setSession(token: string, user: AdminUser): void {
    this.token = token;
    this.user = user;
    try {
      localStorage.setItem(TOKEN_KEY, token);
      localStorage.setItem(USER_KEY, JSON.stringify(user));
    } catch {
      // localStorage 不可用（如隐私模式）时仅内存生效，刷新后需重登
    }
  }

  /** 仅更新用户资料（token 不变），localStorage 同步（改邮箱后调用） */
  updateUser(user: AdminUser): void {
    this.user = user;
    try {
      localStorage.setItem(USER_KEY, JSON.stringify(user));
    } catch {
      // 同 setSession
    }
  }

  logout(): void {
    this.token = null;
    this.user = null;
    localStorage.removeItem(TOKEN_KEY);
    localStorage.removeItem(USER_KEY);
  }
}

export const authStore = new AuthStore();

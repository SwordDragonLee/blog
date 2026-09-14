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
        this.token = data.token;
        this.user = data.user || { id: 0, username };
        localStorage.setItem(TOKEN_KEY, data.token);
        localStorage.setItem(USER_KEY, JSON.stringify(this.user));
      });
    } finally {
      runInAction(() => {
        this.loggingIn = false;
      });
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

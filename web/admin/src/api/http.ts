import axios, { AxiosError } from 'axios';

export const TOKEN_KEY = 'blog_admin_token';

// 统一响应：{ code: 0, message: 'ok', data }
interface ApiEnvelope<T = unknown> {
  code: number;
  message: string;
  data: T;
}

function redirectToLogin() {
  localStorage.removeItem(TOKEN_KEY);
  if (!window.location.pathname.startsWith('/login')) {
    window.location.href = '/login';
  }
}

export const http = axios.create({
  baseURL: '/api/v1',
  timeout: 30000,
});

// 请求拦截：自动附带 JWT
http.interceptors.request.use((config) => {
  const token = localStorage.getItem(TOKEN_KEY);
  if (token) {
    config.headers.Authorization = `Bearer ${token}`;
  }
  return config;
});

// 响应拦截：统一解包 data；code !== 0 或 401 时清 token 跳 /login
http.interceptors.response.use(
  (resp) => {
    const body = resp.data as ApiEnvelope | undefined;
    if (body && typeof body === 'object' && 'code' in body) {
      if (body.code === 401) {
        redirectToLogin();
        return Promise.reject(new Error(body.message || '登录已过期'));
      }
      if (body.code !== 0) {
        return Promise.reject(new Error(body.message || '请求失败'));
      }
      return body.data as never;
    }
    return resp.data as never;
  },
  (error: AxiosError<ApiEnvelope | undefined>) => {
    const status = error.response?.status;
    if (status === 401) {
      redirectToLogin();
    }
    const msg =
      (error.response?.data as ApiEnvelope | undefined)?.message ||
      error.message ||
      '网络错误';
    return Promise.reject(new Error(msg));
  },
);

// 解包后的类型化请求方法（拦截器已返回 data 字段）
export async function get<T>(url: string, params?: object): Promise<T> {
  return http.get(url, { params }) as unknown as Promise<T>;
}

export async function post<T>(
  url: string,
  data?: object,
  config?: object,
): Promise<T> {
  return http.post(url, data, config) as unknown as Promise<T>;
}

export async function put<T>(
  url: string,
  data?: object,
  config?: object,
): Promise<T> {
  return http.put(url, data, config) as unknown as Promise<T>;
}

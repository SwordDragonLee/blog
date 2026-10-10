// 认证与账号（与后端 model.AdminUser 对齐，JSON tag 同名）

export interface AdminUser {
  id: number;
  username: string;
  email?: string;
  created_at?: string;
}

export interface LoginResp {
  token: string;
  user: AdminUser;
}

// 跨业务复用的通用类型

// 统一分页响应 data
export interface PageData<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
}

import axios from 'axios';
import { api } from './authApi';
import type { CheckLinksRequest, CheckLinksResponse, SubmissionRecord } from '@/types';

// 使用统一的 api 实例（已配置认证拦截器）
const linkApiInstance = api;

const normalizeStringArray = (value: unknown): string[] => {
  if (!Array.isArray(value)) {
    return [];
  }

  return value.filter((item): item is string => typeof item === 'string');
};

const normalizeCheckLinksResponse = (data: Partial<CheckLinksResponse>): CheckLinksResponse => ({
  submission_id: typeof data.submission_id === 'number' ? data.submission_id : 0,
  invalid_links: normalizeStringArray(data.invalid_links),
  locked_links: normalizeStringArray(data.locked_links),
  pending_links: normalizeStringArray(data.pending_links),
  valid_links: normalizeStringArray(data.valid_links),
  total_duration: typeof data.total_duration === 'number' ? data.total_duration : undefined,
  invalid_format_count: typeof data.invalid_format_count === 'number' ? data.invalid_format_count : 0,
  duplicate_count: typeof data.duplicate_count === 'number' ? data.duplicate_count : 0,
});

export const linkApi = {
  // 检测链接（公开接口，不需要认证）
  checkLinks: async (data: CheckLinksRequest): Promise<CheckLinksResponse> => {
    // 链接检测接口是公开的，不需要密码头
    const response = await axios.post<CheckLinksResponse>('/api/v1/links/check', data, {
      timeout: 300000, // 5分钟超时，因为实时检测可能需要较长时间
    });
    return normalizeCheckLinksResponse(response.data);
  },

  // 获取提交记录
  getSubmission: async (id: number): Promise<SubmissionRecord> => {
    const response = await linkApiInstance.get<SubmissionRecord>(`/links/submissions/${id}`);
    return response.data;
  },

  // 分页查询提交记录
  listSubmissions: async (page: number = 1, pageSize: number = 20) => {
    const response = await linkApiInstance.get<{
      data: SubmissionRecord[];
      total: number;
      page: number;
      page_size: number;
    }>('/links/submissions', {
      params: { page, page_size: pageSize },
    });
    return response.data;
  },

  // 分页查询被限制的失效链接
  listRateLimitedLinks: async (page: number = 1, pageSize: number = 20, platform?: string) => {
    const params: Record<string, string | number> = { page, page_size: pageSize };
    if (platform) {
      params.platform = platform;
    }
    const response = await linkApiInstance.get<{
      data: Array<{
        id: number;
        link: string;
        platform: string;
        failure_reason: string;
        check_duration?: number;
        is_rate_limited: boolean;
        submission_id?: number;
        created_at: string;
      }>;
      total: number;
      page: number;
      page_size: number;
    }>('/links/rate-limited', { params });
    return response.data;
  },

  // 清空所有被限制的失效链接
  clearRateLimitedLinks: async () => {
    const response = await linkApiInstance.delete<{ message: string }>('/links/rate-limited');
    return response.data;
  },
};


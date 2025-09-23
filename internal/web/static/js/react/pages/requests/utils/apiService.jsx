/**
 * apiService - API服务
 * 文件描述: 提供与后端API通信的服务函数
 * 创建时间: 2025-09-20 18:03:21
 *
 * 主要功能:
 * - RequestsAPI服务类实现
 * - 请求数据获取: /api/v1/usage/requests
 * - 模型列表获取: /api/v1/usage/models
 * - 统计数据获取: /api/v1/usage/stats
 * - 数据导出功能: /api/v1/usage/export
 * - 固定排序: sort_by: 'start_time', sort_order: 'desc'
 * - 完整错误处理和类型检查
 * - SSE实时数据流连接
 */

import { API_ENDPOINTS, ERROR_MESSAGES } from './requestsConstants.jsx';

// 基础请求配置
const DEFAULT_CONFIG = {
    headers: {
        'Content-Type': 'application/json',
    },
    timeout: 30000, // 30秒超时
};

// 创建带有错误处理的fetch包装器
const apiRequest = async (url, options = {}) => {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), DEFAULT_CONFIG.timeout);

    try {
        const response = await fetch(url, {
            ...DEFAULT_CONFIG,
            ...options,
            signal: controller.signal,
        });

        clearTimeout(timeoutId);

        if (!response.ok) {
            let errorMessage = ERROR_MESSAGES.SERVER_ERROR;

            try {
                const errorData = await response.json();
                errorMessage = errorData.message || errorData.error || errorMessage;
            } catch {
                // 如果无法解析错误响应，使用默认错误消息
                errorMessage = `HTTP ${response.status}: ${response.statusText}`;
            }

            throw new Error(errorMessage);
        }

        const contentType = response.headers.get('content-type');
        if (contentType && contentType.includes('application/json')) {
            return await response.json();
        } else {
            return await response.text();
        }
    } catch (error) {
        clearTimeout(timeoutId);

        if (error.name === 'AbortError') {
            throw new Error(ERROR_MESSAGES.REQUEST_TIMEOUT);
        }

        if (error instanceof TypeError && error.message.includes('fetch')) {
            throw new Error(ERROR_MESSAGES.NETWORK_ERROR);
        }

        throw error;
    }
};

/**
 * 获取请求列表数据 - 支持完整的筛选和分页参数
 * @param {Object} params - 查询参数
 * @param {string} [params.status] - 状态筛选
 * @param {string} [params.endpoint] - 端点筛选
 * @param {string} [params.group] - 组筛选
 * @param {string} [params.model] - 模型筛选
 * @param {string} [params.start_date] - 开始时间
 * @param {string} [params.end_date] - 结束时间
 * @param {string} [params.search] - 搜索关键词
 * @param {number} [params.page] - 页码
 * @param {number} [params.limit] - 每页数量
 * @returns {Promise<Object>} 标准化的响应数据
 */
export const fetchRequestsData = async (params = {}) => {
    try {
        const queryParams = new URLSearchParams();

        // 添加所有参数到查询字符串
        Object.entries(params).forEach(([key, value]) => {
            if (value !== null && value !== undefined && value !== '') {
                queryParams.append(key, value.toString());
            }
        });

        // 确保固定排序参数存在
        if (!queryParams.has('sort_by')) {
            queryParams.set('sort_by', 'start_time');
        }
        if (!queryParams.has('sort_order')) {
            queryParams.set('sort_order', 'desc');
        }

        const url = queryParams.toString()
            ? `${API_ENDPOINTS.REQUESTS}?${queryParams.toString()}`
            : API_ENDPOINTS.REQUESTS;

        const data = await apiRequest(url);

        // 字段标准化函数 - 处理API返回的下划线命名到驼峰命名的转换
        const normalizeRequest = (request) => ({
            ...request,
            // 请求ID映射 - 这是最关键的修复
            requestId: request.request_id || request.requestId || request.id,
            id: request.request_id || request.requestId || request.id,

            // 时间字段映射
            timestamp: request.start_time || request.timestamp || request.createdAt,
            startTime: request.start_time || request.startTime,
            updatedAt: request.updated_at || request.updatedAt,

            // 模型、端点、组字段映射（根据原版API）
            model: request.model_name || request.model || request.modelName || 'unknown',
            endpoint: request.endpoint_name || request.endpoint || 'unknown',
            group: request.group_name || request.group || 'default',
            status: request.status,

            // 耗时字段映射（原版API返回duration_ms）
            duration: request.duration_ms || request.duration || 0,

            // 网络字段映射
            method: request.method || 'POST',
            path: request.path || '/v1/messages',
            clientIp: request.client_ip || request.clientIp,
            userAgent: request.user_agent || request.userAgent,
            retryCount: request.retry_count || request.retryCount || 0,
            statusCode: request.status_code || request.statusCode,

            // Token字段映射
            inputTokens: request.input_tokens || request.inputTokens || 0,
            outputTokens: request.output_tokens || request.outputTokens || 0,
            cacheCreationTokens: request.cache_creation_tokens || request.cacheCreationTokens || 0,
            cacheReadTokens: request.cache_read_tokens || request.cacheReadTokens || 0,

            // 成本字段映射（原版API返回total_cost_usd）
            cost: request.total_cost_usd || request.cost || 0,

            // 流式请求标识
            isStreaming: request.is_streaming || request.isStreaming || false,

            // 错误信息映射
            error: request.error_message || request.error || request.errorMessage,
            errorMessage: request.error_message || request.error || request.errorMessage,

            // 请求/响应体映射
            requestBody: request.request_body || request.requestBody,
            responseBody: request.response_body || request.responseBody
        });

        // 标准化响应数据格式，同时应用字段映射
        const requests = data.requests || data.data || data || [];
        const normalizedRequests = Array.isArray(requests) ? requests.map(normalizeRequest) : [];

        return {
            requests: normalizedRequests,
            total: data.total || data.totalCount || data.count || normalizedRequests.length,
            page: data.page || data.currentPage || 1,
            pageSize: data.pageSize || data.limit || 50,
            // 添加元数据支持
            totalPages: data.totalPages || Math.ceil((data.total || 0) / (data.pageSize || data.limit || 50))
        };
    } catch (error) {
        console.error('Failed to fetch requests data:', error);
        throw new Error(`${ERROR_MESSAGES.FETCH_FAILED}: ${error.message}`);
    }
};

// 获取单个请求详情
export const fetchRequestDetail = async (requestId) => {
    try {
        if (!requestId) {
            throw new Error('请求ID不能为空');
        }

        const url = API_ENDPOINTS.REQUEST_DETAIL.replace('{id}', requestId);
        const data = await apiRequest(url);

        return data;
    } catch (error) {
        console.error('Failed to fetch request detail:', error);
        throw new Error(`获取请求详情失败: ${error.message}`);
    }
};

// 获取可用模型列表
export const fetchModels = async () => {
    try {
        const data = await apiRequest(API_ENDPOINTS.MODELS);
        // 支持多种API响应格式：
        // 1. {success: true, data: [...]}
        // 2. {models: [...]}
        // 3. [...]
        let models = [];
        if (data.success && data.data) {
            models = data.data;
        } else if (data.models) {
            models = data.models;
        } else if (Array.isArray(data)) {
            models = data;
        }
        return models;
    } catch (error) {
        console.error('Failed to fetch models:', error);
        throw new Error(`获取模型列表失败: ${error.message}`);
    }
};

// 获取端点列表
export const fetchEndpoints = async () => {
    try {
        const data = await apiRequest(API_ENDPOINTS.ENDPOINTS);
        return data.endpoints || data || [];
    } catch (error) {
        console.error('Failed to fetch endpoints:', error);
        throw new Error(`获取端点列表失败: ${error.message}`);
    }
};

// 获取组列表
export const fetchGroups = async () => {
    try {
        const data = await apiRequest(API_ENDPOINTS.GROUPS);
        return data.groups || data || [];
    } catch (error) {
        console.error('Failed to fetch groups:', error);
        throw new Error(`获取组列表失败: ${error.message}`);
    }
};

// 获取使用统计数据
export const fetchUsageStats = async (params = {}) => {
    try {
        const queryParams = new URLSearchParams();

        // 添加参数到查询字符串
        Object.entries(params).forEach(([key, value]) => {
            if (value !== null && value !== undefined && value !== '') {
                queryParams.append(key, value.toString());
            }
        });

        const url = queryParams.toString()
            ? `${API_ENDPOINTS.STATS}?${queryParams.toString()}`
            : API_ENDPOINTS.STATS;

        const data = await apiRequest(url);
        return data;
    } catch (error) {
        console.error('Failed to fetch usage stats:', error);
        throw new Error(`获取使用统计失败: ${error.message}`);
    }
};

// 删除请求记录
export const deleteRequest = async (requestId) => {
    try {
        if (!requestId) {
            throw new Error('请求ID不能为空');
        }

        const url = API_ENDPOINTS.REQUEST_DETAIL.replace('{id}', requestId);
        const data = await apiRequest(url, {
            method: 'DELETE',
        });

        return data;
    } catch (error) {
        console.error('Failed to delete request:', error);
        throw new Error(`删除请求失败: ${error.message}`);
    }
};

// 批量删除请求记录
export const deleteRequests = async (requestIds) => {
    try {
        if (!requestIds || !Array.isArray(requestIds) || requestIds.length === 0) {
            throw new Error('请求ID列表不能为空');
        }

        const data = await apiRequest(API_ENDPOINTS.REQUESTS, {
            method: 'DELETE',
            body: JSON.stringify({ ids: requestIds }),
        });

        return data;
    } catch (error) {
        console.error('Failed to delete requests:', error);
        throw new Error(`批量删除请求失败: ${error.message}`);
    }
};

// 创建SSE连接用于实时更新
export const createStreamConnection = (onMessage, onError, onOpen) => {
    try {
        const eventSource = new EventSource(API_ENDPOINTS.STREAM);

        eventSource.onopen = (event) => {
            console.log('Stream connection opened');
            if (onOpen) onOpen(event);
        };

        eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                if (onMessage) onMessage(data);
            } catch (error) {
                console.error('Failed to parse stream data:', error);
            }
        };

        eventSource.onerror = (event) => {
            console.error('Stream connection error:', event);
            if (onError) onError(event);
        };

        // 监听特定的事件类型
        eventSource.addEventListener('request_update', (event) => {
            try {
                const data = JSON.parse(event.data);
                if (onMessage) onMessage({ type: 'request_update', data });
            } catch (error) {
                console.error('Failed to parse request update:', error);
            }
        });

        return eventSource;
    } catch (error) {
        console.error('Failed to create stream connection:', error);
        if (onError) onError(error);
        return null;
    }
};

// 关闭SSE连接
export const closeStreamConnection = (eventSource) => {
    if (eventSource && eventSource.readyState !== EventSource.CLOSED) {
        eventSource.close();
    }
};
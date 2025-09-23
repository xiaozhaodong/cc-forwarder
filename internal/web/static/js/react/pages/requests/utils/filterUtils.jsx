/**
 * filterUtils - 筛选器工具函数
 * 文件描述: 提供请求数据筛选和搜索功能的工具函数
 * 创建时间: 2025-09-20 18:03:21
 *
 * 主要筛选功能:
 * - 时间范围计算和筛选
 * - 状态/端点/组/方法筛选
 * - 关键词搜索 (请求ID、端点、组、状态、错误信息)
 * - 自定义时间范围筛选
 * - Token数量范围筛选
 * - 持续时间范围筛选
 * - 模型和流式类型筛选
 * - 成功状态筛选
 * - 筛选条件验证
 * - URL参数构建
 * - 唯一值提取 (端点、组、方法、模型)
 */

import { getTimeRangeConfig } from './requestsConstants.jsx';

// 工具函数：将Date转换为本地时区的时间字符串（解决时区偏差问题）
const toLocalOffsetString = (value) => {
    if (!value) return null;
    const date = new Date(value); // 浏览器会按用户本地时区解析
    if (Number.isNaN(date.getTime())) return null;

    const pad = (num) => String(num).padStart(2, '0');
    const year = date.getFullYear();
    const month = pad(date.getMonth() + 1);
    const day = pad(date.getDate());
    const hours = pad(date.getHours());
    const minutes = pad(date.getMinutes());
    const seconds = pad(date.getSeconds());

    const offsetMinutes = -date.getTimezoneOffset();    // 东八区得到 +480
    const sign = offsetMinutes >= 0 ? '+' : '-';
    const offsetHours = pad(Math.floor(Math.abs(offsetMinutes) / 60));
    const offsetMins = pad(Math.abs(offsetMinutes) % 60);

    return `${year}-${month}-${day}T${hours}:${minutes}:${seconds}${sign}${offsetHours}:${offsetMins}`;
};

// 根据状态筛选请求
export const filterByStatus = (requests, status) => {
    if (!status || status === '') return requests;

    // 如果筛选"error"，包含所有错误类型的状态
    if (status === 'error') {
        const errorStatuses = ['error', 'network_error', 'auth_error', 'rate_limited', 'stream_error'];
        return requests.filter(request => errorStatuses.includes(request.status));
    }

    return requests.filter(request => request.status === status);
};

// 根据端点筛选请求
export const filterByEndpoint = (requests, endpoint) => {
    if (!endpoint || endpoint === '') return requests;
    return requests.filter(request => request.endpoint === endpoint);
};

// 根据组筛选请求
export const filterByGroup = (requests, group) => {
    if (!group || group === '') return requests;
    return requests.filter(request => request.group === group);
};

// 根据HTTP方法筛选请求
export const filterByMethod = (requests, method) => {
    if (!method || method === '') return requests;
    return requests.filter(request =>
        (request.method || 'POST').toUpperCase() === method.toUpperCase()
    );
};

// 根据时间范围筛选请求
export const filterByTimeRange = (requests, timeRange) => {
    if (!timeRange || timeRange === '') return requests;

    const rangeConfig = getTimeRangeConfig(timeRange);
    if (!rangeConfig) return requests;

    const now = Date.now();
    const cutoffTime = now - rangeConfig.value;

    return requests.filter(request => {
        const requestTime = new Date(request.timestamp || request.createdAt).getTime();
        return requestTime >= cutoffTime;
    });
};

// 根据搜索关键词筛选请求
export const filterBySearch = (requests, searchTerm) => {
    if (!searchTerm || searchTerm.trim() === '') return requests;

    const term = searchTerm.toLowerCase().trim();

    return requests.filter(request => {
        // 搜索请求ID
        const requestId = (request.requestId || request.id || '').toLowerCase();
        if (requestId.includes(term)) return true;

        // 搜索端点名称
        const endpoint = (request.endpoint || '').toLowerCase();
        if (endpoint.includes(term)) return true;

        // 搜索组名
        const group = (request.group || '').toLowerCase();
        if (group.includes(term)) return true;

        // 搜索状态
        const status = (request.status || '').toLowerCase();
        if (status.includes(term)) return true;

        // 搜索HTTP方法
        const method = (request.method || 'POST').toLowerCase();
        if (method.includes(term)) return true;

        // 搜索错误信息
        const error = (request.error || '').toLowerCase();
        if (error.includes(term)) return true;

        return false;
    });
};

// 根据自定义时间范围筛选请求
export const filterByCustomTimeRange = (requests, startDate, endDate) => {
    if (!startDate && !endDate) return requests;

    const start = startDate ? new Date(startDate).getTime() : 0;
    const end = endDate ? new Date(endDate).getTime() : Date.now();

    return requests.filter(request => {
        const requestTime = new Date(request.timestamp || request.createdAt).getTime();
        return requestTime >= start && requestTime <= end;
    });
};

// 根据Token数量范围筛选请求
export const filterByTokenRange = (requests, minTokens, maxTokens) => {
    if (!minTokens && !maxTokens) return requests;

    const min = minTokens ? parseInt(minTokens) : 0;
    const max = maxTokens ? parseInt(maxTokens) : Infinity;

    return requests.filter(request => {
        const totalTokens = (request.inputTokens || 0) + (request.outputTokens || 0);
        return totalTokens >= min && totalTokens <= max;
    });
};

// 根据持续时间范围筛选请求
export const filterByDurationRange = (requests, minDuration, maxDuration) => {
    if (!minDuration && !maxDuration) return requests;

    const min = minDuration ? parseFloat(minDuration) : 0;
    const max = maxDuration ? parseFloat(maxDuration) : Infinity;

    return requests.filter(request => {
        let duration = request.duration;

        // 处理不同的duration格式
        if (typeof duration === 'string') {
            if (duration.endsWith('ms')) {
                duration = parseFloat(duration) / 1000;
            } else if (duration.endsWith('s')) {
                duration = parseFloat(duration);
            } else {
                duration = parseFloat(duration);
            }
        } else if (typeof duration === 'number') {
            // 假设是毫秒，转换为秒
            if (duration > 1000) {
                duration = duration / 1000;
            }
        }

        return duration >= min && duration <= max;
    });
};

// 组合所有筛选器
export const filterRequests = (requests, filters) => {
    if (!requests || !Array.isArray(requests)) return [];

    let filteredRequests = [...requests];

    // 应用各种筛选器
    if (filters.status) {
        filteredRequests = filterByStatus(filteredRequests, filters.status);
    }

    if (filters.endpoint) {
        filteredRequests = filterByEndpoint(filteredRequests, filters.endpoint);
    }

    if (filters.group) {
        filteredRequests = filterByGroup(filteredRequests, filters.group);
    }

    if (filters.method) {
        filteredRequests = filterByMethod(filteredRequests, filters.method);
    }

    if (filters.timeRange) {
        filteredRequests = filterByTimeRange(filteredRequests, filters.timeRange);
    }

    if (filters.search) {
        filteredRequests = filterBySearch(filteredRequests, filters.search);
    }

    // 自定义时间范围
    if (filters.startDate || filters.endDate) {
        filteredRequests = filterByCustomTimeRange(
            filteredRequests,
            filters.startDate,
            filters.endDate
        );
    }

    // Token范围筛选
    if (filters.minTokens || filters.maxTokens) {
        filteredRequests = filterByTokenRange(
            filteredRequests,
            filters.minTokens,
            filters.maxTokens
        );
    }

    // 持续时间范围筛选
    if (filters.minDuration || filters.maxDuration) {
        filteredRequests = filterByDurationRange(
            filteredRequests,
            filters.minDuration,
            filters.maxDuration
        );
    }

    return filteredRequests;
};

// 获取唯一的端点列表
export const getUniqueEndpoints = (requests) => {
    if (!requests || !Array.isArray(requests)) return [];

    const endpoints = new Set();
    requests.forEach(request => {
        if (request.endpoint && request.endpoint !== 'unknown') {
            endpoints.add(request.endpoint);
        }
    });

    return Array.from(endpoints).sort();
};

// 获取唯一的组列表
export const getUniqueGroups = (requests) => {
    if (!requests || !Array.isArray(requests)) return [];

    const groups = new Set();
    requests.forEach(request => {
        if (request.group) {
            groups.add(request.group);
        }
    });

    return Array.from(groups).sort();
};

// 获取唯一的HTTP方法列表
export const getUniqueMethods = (requests) => {
    if (!requests || !Array.isArray(requests)) return [];

    const methods = new Set();
    requests.forEach(request => {
        const method = request.method || 'POST';
        methods.add(method.toUpperCase());
    });

    return Array.from(methods).sort();
};

// 根据模型筛选请求
export const filterByModel = (requests, model) => {
    if (!model || model === '') return requests;
    return requests.filter(request =>
        request.model === model || request.modelName === model
    );
};

// 根据流式类型筛选请求
export const filterByStreamingType = (requests, isStreaming) => {
    if (isStreaming === null || isStreaming === undefined) return requests;
    return requests.filter(request => Boolean(request.isStreaming) === Boolean(isStreaming));
};

// 根据成功状态筛选请求
export const filterBySuccess = (requests, isSuccess) => {
    if (isSuccess === null || isSuccess === undefined) return requests;

    if (isSuccess) {
        return requests.filter(request =>
            request.status === 'completed' &&
            (!request.httpStatusCode || request.httpStatusCode >= 200 && request.httpStatusCode < 300)
        );
    } else {
        return requests.filter(request =>
            request.status === 'error' ||
            (request.httpStatusCode && (request.httpStatusCode < 200 || request.httpStatusCode >= 300))
        );
    }
};

// 获取唯一的模型列表
export const getUniqueModels = (requests) => {
    if (!requests || !Array.isArray(requests)) return [];

    const models = new Set();
    requests.forEach(request => {
        const model = request.model || request.modelName;
        if (model && model !== 'unknown') {
            models.add(model);
        }
    });

    return Array.from(models).sort();
};

// 构建筛选器查询参数
export const buildFilterParams = (filters, pagination) => {
    const params = {};

    // 基础筛选器
    if (filters.status) params.status = filters.status;
    if (filters.endpoint) params.endpoint = filters.endpoint;
    if (filters.group) params.group = filters.group;
    if (filters.method) params.method = filters.method;
    if (filters.model) params.model = filters.model;

    // 时间范围
    if (filters.timeRange && filters.timeRange !== 'all') {
        const rangeConfig = getTimeRangeConfig(filters.timeRange);
        if (rangeConfig) {
            const endTime = new Date();
            const startTime = new Date(endTime.getTime() - rangeConfig.value);
            params.start_date = toLocalOffsetString(startTime);
            params.end_date = toLocalOffsetString(endTime);
        }
    }

    // 自定义时间范围
    if (filters.startDate) params.start_date = filters.startDate;
    if (filters.endDate) params.end_date = filters.endDate;

    // 搜索关键词
    if (filters.search) params.search = filters.search;

    // 分页参数
    if (pagination) {
        if (pagination.page) params.page = pagination.page;
        if (pagination.pageSize) params.limit = pagination.pageSize;
        if (pagination.offset !== undefined) params.offset = pagination.offset;
    }

    // 排序参数
    params.sort_by = 'start_time';
    params.sort_order = 'desc';

    return params;
};

// 验证筛选器参数
export const validateFilters = (filters) => {
    const errors = [];

    // 验证日期范围
    if (filters.startDate && filters.endDate) {
        const start = new Date(filters.startDate);
        const end = new Date(filters.endDate);

        if (start > end) {
            errors.push('开始时间不能晚于结束时间');
        }

        if (end > new Date()) {
            errors.push('结束时间不能超过当前时间');
        }
    }

    // 验证Token范围
    if (filters.minTokens && filters.maxTokens) {
        const min = parseInt(filters.minTokens);
        const max = parseInt(filters.maxTokens);

        if (min > max) {
            errors.push('最小Token数不能大于最大Token数');
        }
    }

    // 验证持续时间范围
    if (filters.minDuration && filters.maxDuration) {
        const min = parseFloat(filters.minDuration);
        const max = parseFloat(filters.maxDuration);

        if (min > max) {
            errors.push('最小持续时间不能大于最大持续时间');
        }
    }

    return errors;
};
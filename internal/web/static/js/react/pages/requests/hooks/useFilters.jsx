/**
 * useFilters - 筛选器状态管理Hook
 * 文件描述: 管理请求列表的筛选条件和状态，支持URL同步和筛选验证
 * 创建时间: 2025-09-20 18:03:21
 * 功能: 筛选条件管理、URL同步、筛选验证
 * 筛选条件: timeRange, startDate, endDate, status, model, endpoint, group
 * 状态选项: all, success, failed, timeout, suspended
 */

import React, { useState, useCallback, useMemo, useEffect } from 'react';
import { TIME_RANGES } from '../utils/requestsConstants.jsx';

// 初始筛选器状态
const INITIAL_FILTERS = {
    timeRange: '',          // 时间范围: 1h, 6h, 24h, 7d, 30d
    startDate: '',          // 自定义开始时间
    endDate: '',            // 自定义结束时间
    status: 'all',          // 状态: all, success, failed, timeout, suspended
    model: '',              // 模型筛选（空字符串表示全部模型）
    endpoint: 'all',        // 端点筛选
    group: 'all'            // 组筛选
};

// 状态选项映射
const STATUS_OPTIONS = {
    all: '全部状态',
    success: '成功',
    failed: '失败',
    timeout: '超时',
    suspended: '挂起'
};

export const useFilters = (initialFilters = {}) => {
    const [filters, setFilters] = useState({
        ...INITIAL_FILTERS,
        ...initialFilters
    });

    // 从URL解析筛选参数
    const parseFiltersFromURL = useCallback(() => {
        const urlParams = new URLSearchParams(window.location.search);
        const urlFilters = {};

        // 解析支持的筛选参数
        Object.keys(INITIAL_FILTERS).forEach(key => {
            const value = urlParams.get(key);
            if (value !== null && value !== '') {
                urlFilters[key] = value;
            }
        });

        return urlFilters;
    }, []);

    // 将筛选参数同步到URL
    const syncFiltersToURL = useCallback((newFilters) => {
        const urlParams = new URLSearchParams();

        // 只添加非默认值的筛选参数到URL
        Object.entries(newFilters).forEach(([key, value]) => {
            if (value && value !== '' && value !== 'all' && value !== INITIAL_FILTERS[key]) {
                urlParams.set(key, value);
            }
        });

        // 更新URL，但不刷新页面
        const newURL = urlParams.toString()
            ? `${window.location.pathname}?${urlParams.toString()}`
            : window.location.pathname;

        window.history.replaceState({}, '', newURL);
    }, []);

    // 更新单个筛选器
    const updateFilter = useCallback((key, value) => {
        setFilters(prev => {
            const newFilters = { ...prev, [key]: value };

            // 时间范围特殊处理
            if (key === 'timeRange') {
                if (value && value !== '') {
                    // 使用预设时间范围时，清空自定义时间
                    newFilters.startDate = '';
                    newFilters.endDate = '';
                } else {
                    // 清空时间范围时，也清空自定义时间
                    newFilters.startDate = '';
                    newFilters.endDate = '';
                }
            } else if (key === 'startDate' || key === 'endDate') {
                // 使用自定义时间时，清空预设时间范围
                if (value) {
                    newFilters.timeRange = '';
                }
            }

            // 同步到URL
            syncFiltersToURL(newFilters);
            return newFilters;
        });
    }, [syncFiltersToURL]);

    // 批量更新筛选器
    const updateFilters = useCallback((newFilters) => {
        setFilters(prev => {
            const updated = { ...prev, ...newFilters };
            syncFiltersToURL(updated);
            return updated;
        });
    }, [syncFiltersToURL]);

    // 重置所有筛选器
    const resetFilters = useCallback(() => {
        setFilters(INITIAL_FILTERS);
        syncFiltersToURL(INITIAL_FILTERS);
    }, [syncFiltersToURL]);

    // 应用筛选器 - 生成API查询参数
    const applyFilters = useCallback(() => {
        const queryParams = {};

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

            return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}${sign}${offsetHours}:${offsetMins}`;
        };

        // 处理时间筛选
        if (filters.timeRange && TIME_RANGES[filters.timeRange]) {
            const endTime = new Date();
            const startTime = new Date(endTime.getTime() - TIME_RANGES[filters.timeRange].value);
            const startLocal = toLocalOffsetString(startTime);
            const endLocal = toLocalOffsetString(endTime);
            if (startLocal) queryParams.start_date = startLocal;
            if (endLocal) queryParams.end_date = endLocal;
        } else {
            const startLocal = toLocalOffsetString(filters.startDate);
            const endLocal = toLocalOffsetString(filters.endDate);
            if (startLocal) queryParams.start_date = startLocal;
            if (endLocal) queryParams.end_date = endLocal;
        }

        // 处理状态筛选
        if (filters.status && filters.status !== 'all') {
            const statusMapping = {
                success: 'completed',
                failed: 'error',
                timeout: 'timeout',
                suspended: 'suspended'
            };
            queryParams.status = statusMapping[filters.status] || filters.status;
        }

        // 处理其他筛选条件
        ['model', 'endpoint', 'group'].forEach(key => {
            if (filters[key] && filters[key] !== 'all') {
                queryParams[key] = filters[key];
            }
        });

        return queryParams;
    }, [filters]);

    // 检查是否有活动筛选器
    const hasActiveFilters = useMemo(() => {
        return Object.entries(filters).some(([key, value]) => {
            // 对于每个字段检查是否不等于初始值
            const initialValue = INITIAL_FILTERS[key];
            return value && value !== initialValue && value !== 'all';
        });
    }, [filters]);

    // 获取活动筛选器数量
    const activeFiltersCount = useMemo(() => {
        return Object.entries(filters).filter(([key, value]) => {
            // 对于每个字段检查是否不等于初始值
            const initialValue = INITIAL_FILTERS[key];
            return value && value !== initialValue && value !== 'all';
        }).length;
    }, [filters]);

    // 验证筛选器值
    const validateFilters = useCallback(() => {
        const errors = {};

        // 验证自定义时间范围
        if (filters.startDate && filters.endDate) {
            const start = new Date(filters.startDate);
            const end = new Date(filters.endDate);
            if (start >= end) {
                errors.dateRange = '开始时间必须早于结束时间';
            }
        }

        // 验证时间是否过早
        if (filters.startDate) {
            const start = new Date(filters.startDate);
            const minDate = new Date();
            minDate.setFullYear(minDate.getFullYear() - 1); // 最早1年前
            if (start < minDate) {
                errors.startDate = '开始时间不能早于1年前';
            }
        }

        return {
            isValid: Object.keys(errors).length === 0,
            errors
        };
    }, [filters]);

    // 页面加载时从URL解析筛选参数
    useEffect(() => {
        const urlFilters = parseFiltersFromURL();
        if (Object.keys(urlFilters).length > 0) {
            updateFilters(urlFilters);
        }
    }, [parseFiltersFromURL, updateFilters]);

    return {
        filters,                // 当前筛选器状态
        updateFilter,           // 更新单个筛选器
        updateFilters,          // 批量更新筛选器
        resetFilters,           // 重置所有筛选器
        applyFilters,           // 应用筛选器生成查询参数
        hasActiveFilters,       // 是否有活动筛选器
        activeFiltersCount,     // 活动筛选器数量
        validateFilters,        // 验证筛选器

        // 常量导出，便于组件使用
        STATUS_OPTIONS,         // 状态选项
        TIME_RANGES,           // 时间范围选项
        INITIAL_FILTERS        // 初始筛选器状态
    };
};

// 默认导出主要的Hook（支持两种导入方式）
export default useFilters;
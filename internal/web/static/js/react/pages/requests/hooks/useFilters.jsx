/**
 * useFilters - 筛选器状态管理Hook
 * 文件描述: 管理请求列表的筛选条件和状态，支持URL同步和筛选验证
 * 创建时间: 2025-09-20 18:03:21
 * 功能: 筛选条件管理、URL同步、筛选验证
 * 筛选条件: startDate, endDate, status, model, endpoint, group
 * 状态选项: all, success, failed, timeout, suspended
 */

import React, { useState, useCallback, useMemo, useEffect } from 'react';

// 获取当天的开始和结束时间
const getTodayTimeRange = () => {
    const now = new Date();

    // 当天开始时间 (00:00:00)
    const startOfDay = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 0, 0, 0);

    // 当天结束时间 (23:59:59)
    const endOfDay = new Date(now.getFullYear(), now.getMonth(), now.getDate(), 23, 59, 59);

    // 转换为 datetime-local 格式 (YYYY-MM-DDTHH:mm)
    const formatDateTime = (date) => {
        const year = date.getFullYear();
        const month = String(date.getMonth() + 1).padStart(2, '0');
        const day = String(date.getDate()).padStart(2, '0');
        const hours = String(date.getHours()).padStart(2, '0');
        const minutes = String(date.getMinutes()).padStart(2, '0');
        return `${year}-${month}-${day}T${hours}:${minutes}`;
    };

    return {
        startDate: formatDateTime(startOfDay),
        endDate: formatDateTime(endOfDay)
    };
};

// 初始筛选器状态 - 设置当天时间范围作为默认值
const createInitialFilters = () => {
    const todayRange = getTodayTimeRange();
    return {
        startDate: todayRange.startDate,  // 当天00:00
        endDate: todayRange.endDate,      // 当天23:59
        status: 'all',              // 状态: all, success, failed, timeout, suspended
        model: '',                  // 模型筛选（空字符串表示全部模型）
        endpoint: 'all',            // 端点筛选
        group: 'all'                // 组筛选
    };
};

// 动态获取初始筛选器，避免时间比较问题
const getInitialFilters = () => createInitialFilters();

// 状态选项映射
const STATUS_OPTIONS = {
    all: '全部状态',
    success: '成功',
    failed: '失败',
    timeout: '超时',
    suspended: '挂起'
};

export const useFilters = (initialFilters = {}) => {
    // 使用静态初始状态，避免时间动态更新影响比较
    const staticInitialFilters = useMemo(() => ({
        startDate: '',
        endDate: '',
        status: 'all',
        model: '',
        endpoint: 'all',
        group: 'all'
    }), []);

    const [filters, setFilters] = useState(() => {
        // 只在初始化时使用当天时间作为默认值
        const defaultFilters = createInitialFilters();
        return {
            ...defaultFilters,
            ...initialFilters
        };
    });

    // 从URL解析筛选参数
    const parseFiltersFromURL = useCallback(() => {
        const urlParams = new URLSearchParams(window.location.search);
        const urlFilters = {};

        // 解析支持的筛选参数
        Object.keys(staticInitialFilters).forEach(key => {
            const value = urlParams.get(key);
            if (value !== null && value !== '') {
                urlFilters[key] = value;
            }
        });

        return urlFilters;
    }, [staticInitialFilters]);

    // 将筛选参数同步到URL
    const syncFiltersToURL = useCallback((newFilters) => {
        const urlParams = new URLSearchParams();

        // 只添加非默认值的筛选参数到URL
        Object.entries(newFilters).forEach(([key, value]) => {
            const staticDefault = staticInitialFilters[key];
            if (value && value !== '' && value !== 'all' && value !== staticDefault) {
                urlParams.set(key, value);
            }
        });

        // 更新URL，但不刷新页面
        const newURL = urlParams.toString()
            ? `${window.location.pathname}?${urlParams.toString()}`
            : window.location.pathname;

        window.history.replaceState({}, '', newURL);
    }, [staticInitialFilters]);

    // 更新单个筛选器
    const updateFilter = useCallback((key, value) => {
        setFilters(prev => {
            const newFilters = { ...prev, [key]: value };
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
        // 每次重置时生成新的当天时间范围
        const newInitialFilters = createInitialFilters();
        setFilters(newInitialFilters);
        syncFiltersToURL(newInitialFilters);
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

            return `${year}-${month}-${day}T${hours}:${minutes}:${seconds}${sign}${offsetHours}:${offsetMins}`;
        };

        // 处理时间筛选
        const startLocal = toLocalOffsetString(filters.startDate);
        const endLocal = toLocalOffsetString(filters.endDate);
        if (startLocal) queryParams.start_date = startLocal;
        if (endLocal) queryParams.end_date = endLocal;

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
            // 对于每个字段检查是否不等于静态初始值
            const staticDefault = staticInitialFilters[key];
            // 特殊处理时间字段：只要有设置就认为是活动的
            if (key === 'startDate' || key === 'endDate') {
                return value && value !== '';
            }
            return value && value !== staticDefault && value !== 'all';
        });
    }, [filters, staticInitialFilters]);

    // 获取活动筛选器数量
    const activeFiltersCount = useMemo(() => {
        return Object.entries(filters).filter(([key, value]) => {
            // 对于每个字段检查是否不等于静态初始值
            const staticDefault = staticInitialFilters[key];
            // 特殊处理时间字段：只要有设置就认为是活动的
            if (key === 'startDate' || key === 'endDate') {
                return value && value !== '';
            }
            return value && value !== staticDefault && value !== 'all';
        }).length;
    }, [filters, staticInitialFilters]);

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
        staticInitialFilters   // 静态初始筛选器状态
    };
};

// 默认导出主要的Hook（支持两种导入方式）
export default useFilters;
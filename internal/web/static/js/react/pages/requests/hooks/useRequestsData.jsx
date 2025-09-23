/**
 * useRequestsData - 请求数据管理Hook
 * 文件描述: 管理请求列表数据的获取、缓存、更新和错误处理
 * 创建时间: 2025-09-20 18:03:21
 * 功能: 与 /api/v1/usage/requests API集成，提供完整的数据管理能力
 * 特性: 固定按时间降序排序 (sort_by: 'start_time', sort_order: 'desc')
 */

import React, { useState, useEffect, useCallback, useRef } from 'react';
import { fetchRequestsData, fetchRequestDetail } from '../utils/apiService.jsx';

export const useRequestsData = () => {
    const [requests, setRequests] = useState([]);
    const [totalCount, setTotalCount] = useState(0);
    const [loading, setLoading] = useState(false);
    const [isRefreshing, setIsRefreshing] = useState(false);
    const [hasLoadedOnce, setHasLoadedOnce] = useState(false);
    const [error, setError] = useState(null);
    const [lastUpdated, setLastUpdated] = useState(null);

    // 使用ref避免不必要的重新渲染
    const currentFiltersRef = useRef({});
    const abortControllerRef = useRef(null);

    // 获取请求列表数据 - 支持筛选和分页，区分初次加载和刷新
    const fetchRequests = useCallback(async (filters = {}, pagination = {}, isRefresh = false) => {
        try {
            // 取消之前的请求
            if (abortControllerRef.current) {
                abortControllerRef.current.abort();
            }
            abortControllerRef.current = new AbortController();

            // 根据是否为刷新设置不同的加载状态
            if (isRefresh) {
                setIsRefreshing(true);
            } else {
                setLoading(true);
            }
            setError(null);
            currentFiltersRef.current = { ...filters, ...pagination };

            // 构建API查询参数 - 固定按时间降序排序
            const queryParams = {
                ...filters,
                ...pagination,
                sort_by: 'start_time',      // 固定排序字段
                sort_order: 'desc'          // 固定降序排序
            };

            const data = await fetchRequestsData(queryParams);

            // 检查请求是否被取消
            if (abortControllerRef.current?.signal.aborted) {
                return;
            }

            setRequests(data.requests || []);
            setTotalCount(data.total || 0);
            setLastUpdated(new Date());

            // 标记已加载过数据
            if (!hasLoadedOnce) {
                setHasLoadedOnce(true);
            }
        } catch (err) {
            // 忽略取消的请求错误
            if (err.name === 'AbortError') {
                return;
            }

            setError(err.message || '获取请求数据失败');
            console.error('Failed to fetch requests:', err);
        } finally {
            if (!abortControllerRef.current?.signal.aborted) {
                setLoading(false);
                setIsRefreshing(false);
            }
        }
    }, []);

    // 重新获取数据 - 使用当前筛选条件，以刷新模式执行
    const refetch = useCallback(() => {
        const isRefresh = hasLoadedOnce;
        fetchRequests(currentFiltersRef.current, {}, isRefresh);
    }, [fetchRequests, hasLoadedOnce]);

    // 更新请求状态 - 支持实时更新
    const updateRequest = useCallback((requestId, updates) => {
        setRequests(prev =>
            prev.map(request => {
                const id = request.id || request.requestId || request.request_id;
                if (id === requestId) {
                    return { ...request, ...updates };
                }
                return request;
            })
        );
    }, []);

    // 清理函数
    useEffect(() => {
        return () => {
            if (abortControllerRef.current) {
                abortControllerRef.current.abort();
            }
        };
    }, []);

    return {
        requests,           // 请求列表数据
        totalCount,         // 总记录数
        loading,            // 初次加载状态
        isRefreshing,       // 刷新状态
        hasLoadedOnce,      // 是否已加载过数据
        error,              // 错误信息
        lastUpdated,        // 最后更新时间
        fetchRequests,      // 获取请求数据函数
        refetch,            // 重新获取数据函数
        updateRequest       // 更新单个请求函数
    };
};

// 获取单个请求详情的独立Hook
export const useRequestDetail = () => {
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const fetchDetail = useCallback(async (requestId) => {
        try {
            setLoading(true);
            setError(null);

            if (!requestId) {
                throw new Error('请求ID不能为空');
            }

            const data = await fetchRequestDetail(requestId);
            return data;
        } catch (err) {
            setError(err.message || '获取请求详情失败');
            console.error('Failed to fetch request detail:', err);
            throw err;
        } finally {
            setLoading(false);
        }
    }, []);

    return {
        fetchDetail,
        loading,
        error
    };
};

// 默认导出主要的Hook（支持两种导入方式）
export default useRequestsData;
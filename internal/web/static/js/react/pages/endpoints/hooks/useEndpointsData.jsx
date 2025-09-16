// 端点页面数据管理Hook - 集成SSE实时更新与完整API交互
// 2025-09-15 18:30:00
//
// 功能特性:
// - 完整的端点数据状态管理 (endpoints数组、loading、error状态)
// - SSE实时更新集成 (监听'endpoint'事件类型)
// - 完整的API交互方法 (loadData、updatePriority、performHealthCheck)
// - 详细的调试日志和错误处理
// - SSE连接失败时的定时刷新后备方案
// - 与现有EndpointsManager API完全兼容

import { useState, useCallback, useEffect } from 'react';
import useSSE from '../../../hooks/useSSE.jsx';

// 自定义Hook：端点数据管理 + SSE实时更新
//
// 支持的功能:
// 1. 数据状态管理：endpoints数组、loading、error、initialized状态
// 2. SSE实时更新：处理'endpoint'事件类型，实时同步端点状态变化
// 3. API交互方法：
//    - loadData() - 加载端点数据 (GET /api/v1/endpoints)
//    - updatePriority(endpointName, newPriority) - 更新优先级 (POST /api/v1/endpoints/{name}/priority)
//    - performHealthCheck(endpointName) - 执行健康检测 (POST /api/v1/endpoints/{name}/health-check)
// 4. 错误处理：完善的错误处理和用户反馈
// 5. 后备方案：SSE连接失败时的定时刷新机制
const useEndpointsData = () => {
    const [data, setData] = useState({
        // 端点数据数组 - 提供初始空数组避免undefined
        endpoints: [],

        // 端点统计信息
        total: 0,
        healthy: 0,
        unhealthy: 0,
        unchecked: 0,
        healthPercentage: 0,

        // 状态管理
        loading: false,
        error: null,
        lastUpdate: null
    });

    const [isInitialized, setIsInitialized] = useState(false);

    // SSE端点事件更新处理函数
    const handleSSEUpdate = useCallback((sseData, eventType) => {
        // 🔥 调试：记录所有收到的SSE事件
        console.log('🔍 [端点SSE调试] 收到SSE事件:', {
            eventType,
            sseData,
            hasEventType: !!eventType,
            dataType: sseData?.type,
            hasDataField: !!sseData?.data
        });

        // 只处理端点相关事件
        if (eventType !== 'endpoint') {
            console.log('🚫 [端点SSE调试] 跳过非端点事件:', eventType);
            return; // 不处理非端点事件
        }

        // 🔥 关键修复：从SSE事件结构中解包实际的业务数据
        const actualData = sseData.data || sseData;
        console.log('🎯 [端点SSE] 收到endpoint事件，解包后的数据:', actualData);

        try {
            setData(prevData => {
                const newData = { ...prevData };

                // 处理完整端点数据更新
                if (actualData.endpoints && Array.isArray(actualData.endpoints)) {
                    console.log('📋 [端点SSE] 更新完整端点列表，数量:', actualData.endpoints.length);
                    newData.endpoints = actualData.endpoints;

                    // 重新计算统计信息
                    const stats = calculateEndpointsStats(actualData.endpoints);
                    Object.assign(newData, stats);
                }

                // 处理单个端点状态更新
                else if (actualData.endpoint_name || actualData.name || actualData.endpoint) {
                    const endpointName = actualData.endpoint_name || actualData.name || actualData.endpoint;
                    console.log('🔧 [端点SSE] 更新单个端点状态:', endpointName);

                    newData.endpoints = newData.endpoints.map(endpoint => {
                        if (endpoint.name === endpointName) {
                            // 合并端点数据更新
                            const updatedEndpoint = {
                                ...endpoint,
                                ...actualData
                            };

                            // 移除非端点字段
                            delete updatedEndpoint.endpoint_name;
                            delete updatedEndpoint.endpoint;

                            console.log('✅ [端点SSE] 端点已更新:', endpointName, updatedEndpoint);
                            return updatedEndpoint;
                        }
                        return endpoint;
                    });

                    // 重新计算统计信息
                    const stats = calculateEndpointsStats(newData.endpoints);
                    Object.assign(newData, stats);
                }

                // 处理端点统计更新
                else if (actualData.total !== undefined || actualData.healthy !== undefined) {
                    console.log('📊 [端点SSE] 更新端点统计信息');
                    const statsFields = ['total', 'healthy', 'unhealthy', 'unchecked', 'healthPercentage'];
                    statsFields.forEach(field => {
                        if (actualData[field] !== undefined) {
                            newData[field] = actualData[field];
                        }
                    });
                }

                // 处理通用端点数据字段
                else {
                    console.log('🔄 [端点SSE] 处理通用端点数据更新');
                    // 合并其他端点相关数据
                    Object.keys(actualData).forEach(key => {
                        if (key !== 'eventType' && actualData[key] !== undefined) {
                            newData[key] = actualData[key];
                        }
                    });
                }

                // 更新时间戳
                newData.lastUpdate = new Date().toLocaleTimeString();
                newData.error = null; // 清除之前的错误状态

                return newData;
            });

        } catch (error) {
            console.error('❌ [端点SSE] 事件处理失败:', error, '事件数据:', actualData);
        }
    }, []);

    // 初始化SSE连接
    const { connectionStatus } = useSSE(handleSSEUpdate);

    // 计算端点统计信息的工具函数
    const calculateEndpointsStats = useCallback((endpoints) => {
        if (!endpoints || endpoints.length === 0) {
            return {
                total: 0,
                healthy: 0,
                unhealthy: 0,
                unchecked: 0,
                healthPercentage: 0
            };
        }

        const healthy = endpoints.filter(e => e.healthy && !e.never_checked).length;
        const unhealthy = endpoints.filter(e => !e.healthy && !e.never_checked).length;
        const unchecked = endpoints.filter(e => e.never_checked).length;
        const total = endpoints.length;

        return {
            total,
            healthy,
            unhealthy,
            unchecked,
            healthPercentage: total > 0 ? ((healthy / total) * 100).toFixed(1) : 0
        };
    }, []);

    // 加载端点数据
    const loadData = useCallback(async () => {
        try {
            console.log('🔄 [端点React] 开始加载端点数据...');

            // 只在首次加载时显示loading
            if (!isInitialized) {
                setData(prev => ({ ...prev, loading: true, error: null }));
            }

            const response = await fetch('/api/v1/endpoints');

            if (!response.ok) {
                throw new Error(`API请求失败: ${response.status} ${response.statusText}`);
            }

            const responseData = await response.json();
            console.log('📡 [端点React] API响应数据:', responseData);

            // 处理API响应结构
            const endpoints = responseData.endpoints || [];
            const stats = calculateEndpointsStats(endpoints);

            setData(prevData => ({
                ...prevData,
                endpoints,
                ...stats,
                lastUpdate: new Date().toLocaleTimeString(),
                loading: false,
                error: null
            }));

            setIsInitialized(true);
            console.log('✅ [端点React] 端点数据加载成功, 端点数量:', endpoints.length);

        } catch (error) {
            console.error('❌ [端点React] 端点数据加载失败:', error);
            setData(prev => ({
                ...prev,
                loading: false,
                error: error.message || '端点数据加载失败'
            }));
        }
    }, [isInitialized, calculateEndpointsStats]);

    // 更新端点优先级
    const updatePriority = useCallback(async (endpointName, newPriority) => {
        try {
            console.log('🔧 [端点React] 更新端点优先级:', endpointName, newPriority);

            if (!endpointName) {
                throw new Error('端点名称不能为空');
            }

            if (newPriority < 1) {
                throw new Error('优先级必须大于等于1');
            }

            const response = await fetch(`/api/v1/endpoints/${encodeURIComponent(endpointName)}/priority`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({ priority: parseInt(newPriority) })
            });

            if (!response.ok) {
                throw new Error(`API请求失败: ${response.status} ${response.statusText}`);
            }

            const result = await response.json();
            console.log('📡 [端点React] 优先级更新响应:', result);

            if (result.success) {
                console.log('✅ [端点React] 优先级更新成功:', endpointName, newPriority);

                // 更新本地数据状态
                setData(prevData => ({
                    ...prevData,
                    endpoints: prevData.endpoints.map(endpoint =>
                        endpoint.name === endpointName
                            ? { ...endpoint, priority: parseInt(newPriority) }
                            : endpoint
                    ),
                    lastUpdate: new Date().toLocaleTimeString()
                }));

                // 重新加载数据确保一致性
                setTimeout(() => loadData(), 500);

                return {
                    success: true,
                    message: `端点 ${endpointName} 优先级已更新为 ${newPriority}`
                };
            } else {
                throw new Error(result.error || '更新失败');
            }

        } catch (error) {
            console.error('❌ [端点React] 优先级更新失败:', error);
            return {
                success: false,
                error: error.message || '优先级更新失败'
            };
        }
    }, [loadData]);

    // 执行健康检测
    const performHealthCheck = useCallback(async (endpointName) => {
        try {
            console.log('🏥 [端点React] 执行健康检测:', endpointName);

            if (!endpointName) {
                throw new Error('端点名称不能为空');
            }

            const response = await fetch(`/api/v1/endpoints/${encodeURIComponent(endpointName)}/health-check`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                }
            });

            if (!response.ok) {
                throw new Error(`API请求失败: ${response.status} ${response.statusText}`);
            }

            const result = await response.json();
            console.log('📡 [端点React] 健康检测响应:', result);

            if (result.success) {
                const healthStatus = result.healthy ? '健康' : '不健康';
                console.log('✅ [端点React] 健康检测完成:', endpointName, healthStatus);

                // 更新本地数据状态
                setData(prevData => ({
                    ...prevData,
                    endpoints: prevData.endpoints.map(endpoint =>
                        endpoint.name === endpointName
                            ? {
                                ...endpoint,
                                healthy: result.healthy,
                                never_checked: false,
                                last_check: new Date().toISOString(),
                                response_time: result.response_time || endpoint.response_time
                            }
                            : endpoint
                    ),
                    lastUpdate: new Date().toLocaleTimeString()
                }));

                // 重新计算统计信息
                setData(prevData => ({
                    ...prevData,
                    ...calculateEndpointsStats(prevData.endpoints)
                }));

                // 重新加载数据确保一致性
                setTimeout(() => loadData(), 500);

                return {
                    success: true,
                    healthy: result.healthy,
                    message: `健康检测完成 - ${endpointName}: ${healthStatus}`,
                    response_time: result.response_time
                };
            } else {
                throw new Error(result.error || '健康检测失败');
            }

        } catch (error) {
            console.error('❌ [端点React] 健康检测失败:', error);
            return {
                success: false,
                error: error.message || '健康检测失败'
            };
        }
    }, [loadData, calculateEndpointsStats]);

    // 批量更新多个端点优先级
    const updateMultiplePriorities = useCallback(async (updates) => {
        console.log('🔧 [端点React] 批量更新优先级:', updates);
        const results = [];

        for (const update of updates) {
            const result = await updatePriority(update.name, update.priority);
            results.push({
                endpoint: update.name,
                priority: update.priority,
                ...result
            });
        }

        return results;
    }, [updatePriority]);

    // 批量执行健康检测
    const performBatchHealthCheck = useCallback(async (endpointNames = null) => {
        const targetEndpoints = endpointNames || data.endpoints.map(e => e.name);
        console.log('🏥 [端点React] 批量健康检测:', targetEndpoints);

        const results = [];

        for (const endpointName of targetEndpoints) {
            const result = await performHealthCheck(endpointName);
            results.push({
                endpoint: endpointName,
                ...result
            });
        }

        return results;
    }, [data.endpoints, performHealthCheck]);

    // 搜索端点
    const searchEndpoints = useCallback((query) => {
        if (!query) return data.endpoints;

        const lowerQuery = query.toLowerCase();
        return data.endpoints.filter(endpoint =>
            endpoint.name.toLowerCase().includes(lowerQuery) ||
            endpoint.url.toLowerCase().includes(lowerQuery) ||
            endpoint.group.toLowerCase().includes(lowerQuery)
        );
    }, [data.endpoints]);

    // 按优先级排序端点
    const sortEndpointsByPriority = useCallback((ascending = true) => {
        return [...data.endpoints].sort((a, b) => {
            return ascending ? a.priority - b.priority : b.priority - a.priority;
        });
    }, [data.endpoints]);

    // 按组分组端点
    const getEndpointsByGroup = useCallback(() => {
        const grouped = {};

        data.endpoints.forEach(endpoint => {
            const group = endpoint.group || 'default';
            if (!grouped[group]) {
                grouped[group] = [];
            }
            grouped[group].push(endpoint);
        });

        return grouped;
    }, [data.endpoints]);

    // 获取健康的端点
    const getHealthyEndpoints = useCallback(() => {
        return data.endpoints.filter(endpoint => endpoint.healthy && !endpoint.never_checked);
    }, [data.endpoints]);

    // 获取不健康的端点
    const getUnhealthyEndpoints = useCallback(() => {
        return data.endpoints.filter(endpoint => !endpoint.healthy && !endpoint.never_checked);
    }, [data.endpoints]);

    // 获取未检测的端点
    const getUncheckedEndpoints = useCallback(() => {
        return data.endpoints.filter(endpoint => endpoint.never_checked);
    }, [data.endpoints]);

    // 初始化数据加载
    useEffect(() => {
        // 只在组件挂载时加载一次初始数据
        loadData();
    }, []); // 空依赖数组，只在挂载时执行

    // SSE连接失败时的定时刷新后备方案
    useEffect(() => {
        let interval = null;
        if (connectionStatus === 'failed' || connectionStatus === 'error') {
            console.log('🔄 [端点React] SSE连接失败，启用定时刷新');
            interval = setInterval(loadData, 15000); // 15秒刷新一次
        }

        return () => {
            if (interval) {
                clearInterval(interval);
            }
        };
    }, [connectionStatus, loadData]);

    return {
        // 数据状态
        data,
        endpoints: data.endpoints,
        loading: data.loading,
        error: data.error,
        isInitialized,

        // 统计信息
        stats: {
            total: data.total,
            healthy: data.healthy,
            unhealthy: data.unhealthy,
            unchecked: data.unchecked,
            healthPercentage: data.healthPercentage
        },

        // 核心方法
        loadData,
        refresh: loadData,
        updatePriority,
        performHealthCheck,

        // 批量操作方法
        updateMultiplePriorities,
        performBatchHealthCheck,

        // 数据查询和筛选方法
        searchEndpoints,
        sortEndpointsByPriority,
        getEndpointsByGroup,
        getHealthyEndpoints,
        getUnhealthyEndpoints,
        getUncheckedEndpoints,

        // 系统状态
        sseConnectionStatus: connectionStatus,
        lastUpdate: data.lastUpdate
    };
};

export default useEndpointsData;
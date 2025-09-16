// 概览页面数据管理Hook - 集成SSE实时更新与分离事件处理
// 2025-09-15 17:15:32
//
// 功能特性:
// - 支持分离的SSE事件类型处理 (系统统计 vs 连接统计)
// - 智能事件分类基于 change_type 字段
// - 详细的调试日志和错误处理
// - 向后兼容性支持
// - React hooks最佳实践实现

import React from 'react';
import useSSE from '../../../hooks/useSSE.jsx';

// 自定义Hook：概览数据管理 + SSE实时更新
//
// 支持的事件类型:
// 1. 系统统计事件 (eventType='status' 或 change_type='system_stats_updated')
//    - 处理: uptime, memory_usage, goroutine_count
// 2. 连接统计事件 (eventType='connection' 或 change_type='connection_stats_updated')
//    - 处理: total_requests, active_connections, successful_requests, failed_requests, etc.
// 3. 端点事件 (eventType='endpoint')
// 4. 组管理事件 (eventType='group')
const useOverviewData = () => {
    const [data, setData] = React.useState({
        // 提供初始默认数据，避免undefined导致的闪动
        status: { status: 'running', uptime: '加载中...' },
        endpoints: { total: 0, healthy: 0 },
        connections: {
            total_requests: 0,
            active_connections: 0,
            successful_requests: 0,
            failed_requests: 0,
            average_response_time: '0s',
            total_tokens: 0,
            suspended: {
                suspended_requests: 0,
                total_suspended_requests: 0,
                successful_suspended_requests: 0,
                timeout_suspended_requests: 0,
                success_rate: 0,
                average_suspended_time: '0ms'
            },
            suspended_connections: []
        },
        groups: {
            active_group: null,
            groups: [],
            total_suspended_requests: 0
        },
        lastUpdate: null,
        loading: false,
        error: null
    });

    const [isInitialized, setIsInitialized] = React.useState(false);

    // SSE数据更新处理函数 - 支持分离的事件类型处理
    const handleSSEUpdate = React.useCallback((sseData, eventType) => {
        // 使用解构提取数据，优先从data字段中获取
        // 解构sseData，如果data字段存在就用data，否则回退到sseData本身
        const { data: actualData = sseData } = sseData;
        const { change_type: changeType } = actualData;

        console.log(`📡 [概览SSE] 收到${eventType || 'generic'}事件, 变更类型: ${changeType || 'none'}`, sseData);

        try {
            setData(prevData => {
                const newData = { ...prevData };

                // 1. 处理系统统计事件 - 只处理系统级数据
                if (eventType === 'status' || changeType === 'system_stats_updated') {
                    console.log('🖥️ [概览SSE] 处理系统统计事件');

                    const systemFields = ['uptime', 'memory_usage', 'goroutine_count'];
                    const systemUpdates = {};

                    // 提取系统级字段
                    systemFields.forEach(field => {
                        if (sseData[field] !== undefined) {
                            systemUpdates[field] = sseData[field];
                        }
                    });

                    // 处理嵌套的 status 对象
                    if (sseData.status) {
                        Object.assign(systemUpdates, sseData.status);
                    }

                    if (Object.keys(systemUpdates).length > 0) {
                        newData.status = { ...newData.status, ...systemUpdates };
                        console.log('✅ [概览SSE] 系统统计已更新:', systemUpdates);
                    }
                }

                // 2. 处理连接统计事件 - 处理连接统计数据
                if (eventType === 'connection' || changeType === 'connection_stats_updated') {
                    console.log('🔗 [概览SSE] 处理连接统计事件');

                    // 直接连接统计字段
                    const connectionFields = [
                        'total_requests', 'active_connections', 'successful_requests',
                        'failed_requests', 'average_response_time', 'total_tokens',
                        'total_suspended_requests'
                    ];

                    const connectionUpdates = {};
                    connectionFields.forEach(field => {
                        if (actualData[field] !== undefined) {
                            connectionUpdates[field] = actualData[field];
                        }
                    });

                    // 处理嵌套的 connections 对象
                    if (actualData.connections) {
                        Object.assign(connectionUpdates, actualData.connections);
                    }

                    // 处理挂起请求统计
                    if (actualData.suspended) {
                        newData.connections.suspended = {
                            ...newData.connections.suspended,
                            ...actualData.suspended
                        };
                        console.log('📋 [概览SSE] 挂起请求统计已更新:', actualData.suspended);
                    }

                    // 处理挂起连接列表
                    if (actualData.suspended_connections) {
                        newData.connections.suspended_connections = actualData.suspended_connections;
                        console.log('📃 [概览SSE] 挂起连接列表已更新, 数量:', actualData.suspended_connections.length);
                    }

                    if (Object.keys(connectionUpdates).length > 0) {
                        newData.connections = { ...newData.connections, ...connectionUpdates };
                        console.log('✅ [概览SSE] 连接统计已更新:', connectionUpdates);
                    }
                }

                // 3. 处理端点事件 - 保持向后兼容
                if (eventType === 'endpoint' || sseData.endpoints) {
                    console.log('🎯 [概览SSE] 处理端点事件');
                    newData.endpoints = { ...newData.endpoints, ...(sseData.endpoints || sseData) };
                }

                // 4. 处理组事件 - 保持向后兼容
                if (eventType === 'group' || sseData.groups) {
                    console.log('👥 [概览SSE] 处理组事件');
                    newData.groups = { ...newData.groups, ...(sseData.groups || sseData) };
                }

                // 5. 通用字段处理 - 向后兼容性支持
                if (!changeType && (eventType === 'status' || sseData.status)) {
                    console.log('🔄 [概览SSE] 向后兼容 - 处理通用状态事件');
                    newData.status = { ...newData.status, ...(sseData.status || sseData) };
                }

                // 更新时间戳
                newData.lastUpdate = new Date().toLocaleTimeString();

                return newData;
            });
        } catch (error) {
            console.error('❌ [概览SSE] 事件处理失败:', error, '事件数据:', sseData);
        }
    }, []);

    // 初始化SSE连接
    const { connectionStatus } = useSSE(handleSSEUpdate);

    const loadData = React.useCallback(async () => {
        try {
            console.log('🔄 [概览React] 开始加载数据...');

            // 只在首次加载时显示loading
            if (!isInitialized) {
                setData(prev => ({ ...prev, loading: true, error: null }));
            }

            const [statusResponse, endpointsResponse, connectionsResponse, groupsResponse] = await Promise.all([
                fetch('/api/v1/status'),
                fetch('/api/v1/endpoints'),
                fetch('/api/v1/connections'),
                fetch('/api/v1/groups')
            ]);

            if (!statusResponse.ok || !endpointsResponse.ok || !connectionsResponse.ok || !groupsResponse.ok) {
                throw new Error('API请求失败');
            }

            const [status, endpoints, connections, groups] = await Promise.all([
                statusResponse.json(),
                endpointsResponse.json(),
                connectionsResponse.json(),
                groupsResponse.json()
            ]);

            // 数据合并，保持原有结构，避免字段丢失
            setData(prevData => ({
                status: { ...prevData.status, ...status },
                endpoints: { ...prevData.endpoints, ...endpoints },
                connections: {
                    ...prevData.connections,
                    ...connections,
                    suspended: { ...prevData.connections.suspended, ...connections.suspended }
                },
                groups: { ...prevData.groups, ...groups },
                lastUpdate: new Date().toLocaleTimeString(),
                loading: false,
                error: null
            }));

            setIsInitialized(true);
            console.log('✅ [概览React] 数据加载成功');
        } catch (error) {
            console.error('❌ [概览React] 数据加载失败:', error);
            setData(prev => ({
                ...prev,
                loading: false,
                error: error.message || '数据加载失败'
            }));
        }
    }, [isInitialized]);

    React.useEffect(() => {
        // 只在组件挂载时加载一次初始数据
        loadData();
    }, []); // 空依赖数组，只在挂载时执行

    React.useEffect(() => {
        // 如果SSE连接失败，则使用定时刷新作为后备
        let interval = null;
        if (connectionStatus === 'failed' || connectionStatus === 'error') {
            console.log('🔄 [概览React] SSE连接失败，启用定时刷新');
            interval = setInterval(loadData, 10000); // 10秒刷新一次
        }

        return () => {
            if (interval) {
                clearInterval(interval);
            }
        };
    }, [connectionStatus, loadData]);

    return {
        data,
        loadData,
        refresh: loadData,
        isInitialized,
        sseConnectionStatus: connectionStatus
    };
};

export default useOverviewData;
/**
 * Requests Page - 请求管理页面主组件
 * 文件描述: 完整集成所有子组件的请求管理页面，支持筛选、分页、详情查看等功能
 * 创建时间: 2025-09-20 19:23:22
 *
 * 功能特性:
 * - 完整的组件集成 (FiltersPanel, RequestsTable, RequestDetailModal等)
 * - Hook集成 (useRequestsData, useFilters, usePagination)
 * - 状态管理协调和事件处理
 * - 响应式布局和无障碍支持
 * - 100%功能兼容性（除简化的排序和暂缓的SSE外）
 */

import React, { useState, useCallback, useEffect, useMemo } from 'react';
import FiltersPanel from './components/FiltersPanel.jsx';
import RequestsTable from './components/RequestsTable.jsx';
import RequestDetailModal from './components/RequestDetailModal.jsx';
import StatsOverview from './components/StatsOverview.jsx';
import useRequestsData from './hooks/useRequestsData.jsx';
import useFilters from './hooks/useFilters.jsx';
import usePagination from './hooks/usePagination.jsx';
import { fetchUsageStats } from './utils/apiService.jsx';

const RequestsPage = () => {
    // 数据管理Hook
    const {
        requests,
        totalCount,
        loading,
        isRefreshing,
        hasLoadedOnce,
        error,
        fetchRequests,
        refetch
    } = useRequestsData();

    // 筛选器Hook
    const {
        filters,
        updateFilter,
        resetFilters,
        applyFilters,
        hasActiveFilters,
        activeFiltersCount,
        validateFilters,
        STATUS_OPTIONS  // 导入更新后的状态选项
    } = useFilters();

    // 分页Hook
    const pagination = usePagination(totalCount);

    // 详情模态框状态
    const [selectedRequest, setSelectedRequest] = useState(null);
    const [isModalOpen, setIsModalOpen] = useState(false);

    // 统计数据状态
    const [statsData, setStatsData] = useState({
        totalRequests: 0,
        successRate: '-%',
        avgDuration: '-',
        totalCost: '$0.00',
        totalTokens: '0',
        failedRequests: 0
    });
    const [statsLoading, setStatsLoading] = useState(false);
    const [hasStatsLoaded, setHasStatsLoaded] = useState(false);
    const [isStatsRefreshing, setIsStatsRefreshing] = useState(false);

    // 注释掉详情Hook，直接使用列表数据
    // const { fetchDetail, loading: detailLoading, error: detailError } = useRequestDetail();

    // 加载统计数据
    const loadStatsData = useCallback(async () => {
        try {
            // 区分首次加载和刷新
            if (!hasStatsLoaded) {
                setStatsLoading(true);
            } else {
                setIsStatsRefreshing(true);
            }

            const queryParams = applyFilters(); // 使用相同的筛选条件
            const response = await fetchUsageStats(queryParams);

            // 解构后端返回的数据格式：{success: true, data: {...}}
            const data = response?.data || response;

            // 格式化统计数据（参考原版实现）
            const formatTokens = (tokens) => {
                const numericTokens = Number(tokens) || 0;
                if (numericTokens === 0) return '0.00M';

                // 转换为百万单位，与原版保持一致
                const tokensInM = numericTokens / 1000000;

                if (tokensInM >= 1) {
                    return `${tokensInM.toFixed(2)}M`;
                } else {
                    return `${tokensInM.toFixed(3)}M`;
                }
            };

            const formatCost = (cost) => {
                if (!cost || cost === 0) return '$0.00';
                const num = parseFloat(cost);
                if (isNaN(num)) return '$0.00';
                return `$${num.toFixed(2)}`;  // 保留2位小数，四舍五入
            };

            const formatDuration = (duration) => {
                if (!duration || duration === 0) return '-';
                const ms = parseFloat(duration);
                if (isNaN(ms)) return '-';
                if (ms >= 1000) {
                    return `${(ms / 1000).toFixed(1)}s`;
                } else {
                    return `${Math.round(ms)}ms`;
                }
            };

            setStatsData({
                totalRequests: data.total_requests || 0,
                successRate: data.success_rate ? `${data.success_rate.toFixed(1)}%` : '-%',
                avgDuration: formatDuration(data.avg_duration_ms),
                totalCost: formatCost(data.total_cost_usd),
                totalTokens: formatTokens(data.total_tokens),
                failedRequests: data.failed_requests || 0  // 修正字段名
            });

            // 标记已加载过统计数据
            if (!hasStatsLoaded) {
                setHasStatsLoaded(true);
            }
        } catch (error) {
            console.error('加载统计数据失败:', error);
            // 保持默认值，不显示错误
        } finally {
            if (!hasStatsLoaded) {
                setStatsLoading(false);
            } else {
                setIsStatsRefreshing(false);
            }
        }
    }, [applyFilters, hasStatsLoaded]);

    // 加载数据的统一函数
    const loadData = useCallback((forceRefresh = false, skipValidation = false) => {
        // 重置操作时跳过验证（因为重置是恢复到有效的初始状态）
        if (!skipValidation) {
            const { isValid, errors } = validateFilters();
            if (!isValid) {
                // 使用通知系统显示验证错误
                Object.entries(errors).forEach(([field, message]) => {
                    window.Utils.showWarning(message);
                });
                return;
            }
        }

        const queryParams = applyFilters();
        const paginationParams = pagination.getPaginationParams();

        // 判断是否为刷新模式：已经加载过数据且不是强制刷新
        const isRefresh = hasLoadedOnce && !forceRefresh;

        // 同时加载请求数据和统计数据
        fetchRequests(queryParams, paginationParams, isRefresh);
        loadStatsData();
    }, [applyFilters, pagination, fetchRequests, validateFilters, loadStatsData, hasLoadedOnce]);

    // 处理筛选器应用
    const handleApplyFilters = useCallback((queryParams) => {
        console.log('应用筛选条件:', filters, '查询参数:', queryParams);
        // 应用筛选时重置到第一页，强制重新加载（不使用刷新模式）
        pagination.resetPagination();
        loadData(true); // forceRefresh = true
    }, [filters, pagination, loadData]);

    // 处理筛选器重置
    const handleResetFilters = useCallback(() => {
        console.log('重置筛选条件');
        resetFilters();
        pagination.resetPagination();
        // 延迟加载以确保状态更新完成，强制重新加载，跳过验证
        setTimeout(() => loadData(true, true), 0); // forceRefresh = true, skipValidation = true
    }, [resetFilters, pagination, loadData]);

    // 处理行点击事件 - 直接使用列表数据，不调用详情API
    const handleRowClick = useCallback(async (request) => {
        try {
            setSelectedRequest(request);
            setIsModalOpen(true);

            // 注释掉详情API调用，直接使用列表数据
            // 列表数据现在已经包含 failure_reason, cancel_reason, last_failure_reason 等新字段
            /*
            // 如果请求数据不完整，获取详细信息
            const requestId = request.id || request.requestId || request.request_id;
            if (requestId && (!request.requestBody || !request.responseBody)) {
                const detailData = await fetchDetail(requestId);
                setSelectedRequest(detailData);
            }
            */
        } catch (err) {
            console.error('显示请求详情失败:', err);
            // 即使出错，也显示基本信息
            setSelectedRequest(request);
            setIsModalOpen(true);
        }
    }, []);

    // 关闭模态框
    const handleCloseModal = useCallback(() => {
        setIsModalOpen(false);
        setSelectedRequest(null);
    }, []);

    // 处理刷新
    const handleRefresh = useCallback(() => {
        console.log('刷新请求数据');
        loadData();
    }, [loadData]);

    // 初始数据加载
    useEffect(() => {
        loadData();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []); // 只在组件挂载时执行一次

    // 监听分页变化
    useEffect(() => {
        if (pagination.currentPage > 1 || (requests.length > 0 && pagination.currentPage === 1)) {
            loadData();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [pagination.currentPage, pagination.pageSize]);

    // 键盘事件处理 (无障碍支持)
    useEffect(() => {
        const handleKeyDown = (event) => {
            // Escape键关闭模态框
            if (event.key === 'Escape' && isModalOpen) {
                handleCloseModal();
            }
            // F5刷新数据
            if (event.key === 'F5') {
                event.preventDefault();
                handleRefresh();
            }
        };

        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isModalOpen, handleCloseModal, handleRefresh]);

    return (
        <div className="requests-page">
            <div className="section">
                <h2>📊 请求追踪</h2>

                {/* 筛选面板 - 传递筛选器状态和方法 */}
                <FiltersPanel
                    onApplyFilters={handleApplyFilters}
                    onResetFilters={handleResetFilters}
                    filters={filters}
                    updateFilter={updateFilter}
                    resetFilters={resetFilters}
                    applyFilters={applyFilters}
                    hasActiveFilters={hasActiveFilters}
                    activeFiltersCount={activeFiltersCount}
                    STATUS_OPTIONS={STATUS_OPTIONS}  // 使用从Hook导入的状态选项
                />

                {/* 统计概览卡片 */}
                <StatsOverview
                    stats={statsData}
                    isLoading={statsLoading}
                    isRefreshing={isStatsRefreshing}
                />

                {/* 主要内容区域 */}
                <div className="requests-content">
                    {error ? (
                        <div className="error-message">
                            <div className="error-icon">❌</div>
                            <div className="error-content">
                                <h3>加载失败</h3>
                                <p>加载请求数据时出错: {error}</p>
                                <button
                                    className="btn btn-primary"
                                    onClick={handleRefresh}
                                >
                                    🔄 重试
                                </button>
                            </div>
                        </div>
                    ) : (
                        <RequestsTable
                            requests={requests}
                            isInitialLoading={loading && !hasLoadedOnce}
                            onRowClick={handleRowClick}
                            pagination={pagination}
                            onRefresh={handleRefresh}
                        />
                    )}
                </div>

                {/* 请求详情模态框 - 兼容现有接口 */}
                <RequestDetailModal
                    request={selectedRequest}
                    isOpen={isModalOpen}
                    onClose={handleCloseModal}
                />
            </div>
        </div>
    );
};

// 组件 memo 优化，避免不必要的重新渲染
const MemoizedRequestsPage = React.memo(RequestsPage);

// 为开发工具提供组件名称
MemoizedRequestsPage.displayName = 'RequestsPage';

export default MemoizedRequestsPage;
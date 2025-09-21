/**
 * FiltersPanel - 筛选面板组件
 * 文件描述: 提供请求数据的多维度筛选功能，支持时间范围、状态、模型、端点、组等筛选条件
 * 创建时间: 2025-09-20 18:03:21
 *
 * 功能特性:
 * - 时间范围筛选 (预设: 1h, 6h, 24h, 7d, 30d)
 * - 自定义时间范围 (datetime-local 输入)
 * - 状态筛选 (all, success, failed, timeout, suspended)
 * - 模型筛选 (动态从 /api/v1/usage/models 加载)
 * - 端点筛选 (动态选项)
 * - 组筛选 (动态选项)
 * - 筛选条件重置功能
 * - 与 useFilters Hook 集成
 */

import React, { useState, useEffect } from 'react';
import { TIME_RANGES } from '../utils/requestsConstants.jsx';
import { fetchModels, fetchEndpoints, fetchGroups } from '../utils/apiService.jsx';

const FiltersPanel = ({
    onApplyFilters,
    onResetFilters,
    filters,
    updateFilter,
    resetFilters,
    applyFilters,
    hasActiveFilters,
    activeFiltersCount,
    STATUS_OPTIONS
}) => {
    // 动态选项状态
    const [dynamicOptions, setDynamicOptions] = useState({
        models: [],
        endpoints: [],
        groups: [],
        isLoading: true
    });

    // 是否显示自定义时间选择器
    const [showCustomDateRange, setShowCustomDateRange] = useState(
        filters.timeRange === 'custom' || (!filters.timeRange && (filters.startDate || filters.endDate))
    );

    // 初始化加载动态选项数据
    useEffect(() => {
        const loadDynamicOptions = async () => {
            try {
                setDynamicOptions(prev => ({ ...prev, isLoading: true }));

                const [modelsData, endpointsData, groupsData] = await Promise.all([
                    fetchModels().catch(err => {
                        console.warn('获取模型列表失败:', err);
                        return [];
                    }),
                    fetchEndpoints().catch(err => {
                        console.warn('获取端点列表失败:', err);
                        return [];
                    }),
                    fetchGroups().catch(err => {
                        console.warn('获取组列表失败:', err);
                        return [];
                    })
                ]);

                setDynamicOptions({
                    models: Array.isArray(modelsData) ? modelsData : [],
                    endpoints: Array.isArray(endpointsData) ? endpointsData : [],
                    groups: Array.isArray(groupsData) ? groupsData : [],
                    isLoading: false
                });
            } catch (error) {
                console.error('加载动态选项失败:', error);
                setDynamicOptions(prev => ({ ...prev, isLoading: false }));
            }
        };

        loadDynamicOptions();
    }, []);

    // 监听时间范围变化
    useEffect(() => {
        setShowCustomDateRange(
            filters.timeRange === 'custom' || (!filters.timeRange && (filters.startDate || filters.endDate))
        );
    }, [filters.timeRange, filters.startDate, filters.endDate]);

    // 处理时间范围筛选器变化
    const handleTimeRangeChange = (value) => {
        updateFilter('timeRange', value);
        if (value === 'custom') {
            setShowCustomDateRange(true);
        } else {
            setShowCustomDateRange(false);
        }
    };

    // 处理筛选器应用
    const handleApplyFilters = () => {
        const queryParams = applyFilters();
        if (onApplyFilters) {
            onApplyFilters(queryParams);
        }
    };

    // 处理筛选器重置
    const handleResetFilters = () => {
        resetFilters();
        setShowCustomDateRange(false);
        if (onResetFilters) {
            onResetFilters();
        }
    };

    return (
        <div className="filters-panel">
            <div className="filters-grid">
                {/* 时间范围筛选 */}
                <div className="filter-group">
                    <label>时间范围:</label>
                    <select
                        id="time-range-filter"
                        value={filters.timeRange || ''}
                        onChange={(e) => handleTimeRangeChange(e.target.value)}
                    >
                        <option value="">全部时间</option>
                        {Object.entries(TIME_RANGES).map(([key, config]) => {
                            if (key !== 'all') {
                                return (
                                    <option key={key} value={key}>
                                        最近{config.label}
                                    </option>
                                );
                            }
                            return null;
                        })}
                        <option value="custom">自定义</option>
                    </select>
                </div>

                {/* 自定义时间范围 */}
                {showCustomDateRange && (
                    <div className="filter-group custom-time-range" id="custom-date-range">
                        <label>自定义时间:</label>
                        <div className="datetime-inputs">
                            <div className="datetime-field">
                                <input
                                    type="datetime-local"
                                    id="start-date"
                                    className="filter-input"
                                    value={filters.startDate || ''}
                                    onChange={(e) => updateFilter('startDate', e.target.value)}
                                    placeholder="开始时间"
                                />
                            </div>
                            <span className="datetime-separator">至</span>
                            <div className="datetime-field">
                                <input
                                    type="datetime-local"
                                    id="end-date"
                                    className="filter-input"
                                    value={filters.endDate || ''}
                                    onChange={(e) => updateFilter('endDate', e.target.value)}
                                    placeholder="结束时间"
                                />
                            </div>
                        </div>
                    </div>
                )}

                {/* 状态筛选 */}
                <div className="filter-group">
                    <label>状态:</label>
                    <select
                        id="status-filter"
                        value={filters.status || 'all'}
                        onChange={(e) => updateFilter('status', e.target.value)}
                    >
                        {Object.entries(STATUS_OPTIONS).map(([value, label]) => (
                            <option key={value} value={value}>
                                {label}
                            </option>
                        ))}
                    </select>
                </div>

                {/* 模型筛选 */}
                <div className="filter-group">
                    <label>模型:</label>
                    <select
                        id="model-filter"
                        value={filters.model || ''}
                        onChange={(e) => updateFilter('model', e.target.value)}
                        disabled={dynamicOptions.isLoading}
                    >
                        <option value="">全部模型</option>
                        {dynamicOptions.models.map((model) => {
                            // 支持两种数据格式：对象{model_name, display_name}或字符串
                            const modelName = typeof model === 'string' ? model : model.model_name;
                            return (
                                <option key={modelName} value={modelName}>
                                    {modelName}
                                </option>
                            );
                        })}
                    </select>
                </div>

                {/* 端点筛选 */}
                <div className="filter-group">
                    <label>端点:</label>
                    <select
                        id="endpoint-filter"
                        value={filters.endpoint || 'all'}
                        onChange={(e) => updateFilter('endpoint', e.target.value)}
                        disabled={dynamicOptions.isLoading}
                    >
                        <option value="all">全部端点</option>
                        {dynamicOptions.endpoints.map((endpoint) => {
                            // 支持两种数据格式：对象{name, ...}或字符串
                            const endpointName = typeof endpoint === 'string' ? endpoint : (endpoint.name || endpoint.endpoint_name);
                            const displayName = typeof endpoint === 'string' ? endpoint : (endpoint.display_name || endpoint.name || endpoint.endpoint_name);
                            return (
                                <option key={endpointName} value={endpointName}>
                                    {displayName}
                                </option>
                            );
                        })}
                    </select>
                </div>

                {/* 组筛选 */}
                <div className="filter-group">
                    <label>组:</label>
                    <select
                        id="group-filter"
                        value={filters.group || 'all'}
                        onChange={(e) => updateFilter('group', e.target.value)}
                        disabled={dynamicOptions.isLoading}
                    >
                        <option value="all">全部组</option>
                        {dynamicOptions.groups.map((group) => {
                            // 支持两种数据格式：对象{name, ...}或字符串
                            const groupName = typeof group === 'string' ? group : (group.name || group.group_name);
                            const displayName = typeof group === 'string' ? group : (group.display_name || group.name || group.group_name);
                            return (
                                <option key={groupName} value={groupName}>
                                    {displayName}
                                </option>
                            );
                        })}
                    </select>
                </div>

                {/* 操作按钮 */}
                <div className="filter-actions">
                    <button
                        className="btn btn-primary"
                        onClick={handleApplyFilters}
                        title={`应用筛选条件${hasActiveFilters ? ` (${activeFiltersCount}个活动筛选器)` : ''}`}
                    >
                        🔍 搜索
                        {hasActiveFilters && (
                            <span style={{
                                marginLeft: '4px',
                                backgroundColor: 'rgba(255,255,255,0.3)',
                                borderRadius: '50%',
                                padding: '2px 6px',
                                fontSize: '11px'
                            }}>
                                {activeFiltersCount}
                            </span>
                        )}
                    </button>
                    <button
                        className="btn btn-secondary"
                        onClick={handleResetFilters}
                        disabled={!hasActiveFilters}
                        title="清除所有筛选条件"
                    >
                        🔄 重置
                    </button>
                </div>
            </div>

            {/* 加载状态指示器 */}
            {dynamicOptions.isLoading && (
                <div style={{
                    marginTop: '10px',
                    fontSize: '12px',
                    color: '#6b7280',
                    textAlign: 'center'
                }}>
                    正在加载筛选选项...
                </div>
            )}
        </div>
    );
};

export default FiltersPanel;

/**
 * 端点页面主组件 (入口组件)
 *
 * 负责：
 * - 端点管理页面的主要入口
 * - 协调各个子组件的渲染
 * - 管理页面整体状态和布局
 * - 集成数据管理和错误处理
 * - 保持与原版本完全一致的HTML结构
 *
 * 创建日期: 2025-09-15 23:47:50
 * 完整实现日期: 2025-09-16
 * @author Claude Code Assistant
 */

import React from 'react';
import useEndpointsData from './hooks/useEndpointsData.jsx';
import EndpointsTable from './components/EndpointsTable.jsx';

/**
 * 端点页面主组件
 *
 * 功能特性：
 * - 使用 useEndpointsData Hook 管理数据状态
 * - 集成 EndpointsTable 组件显示端点列表
 * - 处理 loading、error、empty 等各种状态
 * - 提供友好的错误恢复机制
 * - 保持与原版本完全一致的HTML结构和CSS类名
 *
 * @returns {JSX.Element} 端点页面JSX元素
 */
const EndpointsPage = () => {
    // 获取端点数据和操作方法
    const {
        data,
        loading,
        error,
        updatePriority,
        performHealthCheck,
        refresh
    } = useEndpointsData();

    console.log('📋 [端点页面] 页面渲染状态:', {
        loading,
        error,
        endpointsCount: data?.endpoints?.length || 0,
        hasData: !!data?.endpoints
    });

    // 错误状态渲染 - 提供友好的错误UI和重试机制
    if (error) {
        console.log('❌ [端点页面] 渲染错误状态:', error);
        return (
            <div className="section">
                <h2>📡 端点状态</h2>
                <div style={{
                    textAlign: 'center',
                    padding: '48px 24px',
                    color: '#ef4444',
                    border: '1px solid #fecaca',
                    borderRadius: '8px',
                    backgroundColor: '#fef2f2'
                }}>
                    <div style={{ fontSize: '48px', marginBottom: '16px' }}>❌</div>
                    <h3 style={{
                        margin: '0 0 8px 0',
                        color: '#dc2626',
                        fontSize: '18px',
                        fontWeight: '600'
                    }}>端点数据加载失败</h3>
                    <p style={{
                        margin: '0 0 16px 0',
                        color: '#7f1d1d',
                        fontSize: '14px',
                        lineHeight: '1.5'
                    }}>{error}</p>
                    <button
                        onClick={refresh}
                        className="btn btn-primary"
                        style={{
                            backgroundColor: '#3b82f6',
                            color: 'white',
                            border: 'none',
                            borderRadius: '6px',
                            padding: '8px 16px',
                            fontSize: '14px',
                            cursor: 'pointer',
                            transition: 'background-color 0.2s'
                        }}
                        onMouseOver={(e) => e.target.style.backgroundColor = '#2563eb'}
                        onMouseOut={(e) => e.target.style.backgroundColor = '#3b82f6'}
                    >
                        🔄 重试
                    </button>
                </div>
            </div>
        );
    }

    // 主要内容渲染 - 与原始版本结构完全一致
    console.log('✅ [端点页面] 渲染正常状态, 端点数量:', data?.endpoints?.length || 0);
    return (
        <div className="section">
            <h2>📡 端点状态</h2>
            <div id="endpoints-table">
                <EndpointsTable
                    endpoints={data?.endpoints || []}
                    loading={loading}
                    onUpdatePriority={updatePriority}
                    onHealthCheck={performHealthCheck}
                    onRefresh={refresh}
                />
            </div>
        </div>
    );
};

export default EndpointsPage;
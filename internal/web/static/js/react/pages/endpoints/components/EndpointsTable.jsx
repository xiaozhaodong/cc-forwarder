/**
 * 端点表格组件 (表格容器)
 *
 * 负责：
 * - 渲染端点数据表格的容器结构
 * - 管理表格的整体布局和样式
 * - 协调各个端点行组件的显示
 * - 处理表格级别的交互逻辑
 * - 处理加载状态、空数据状态和正常显示状态
 *
 * 创建日期: 2025-09-15 23:47:50
 * 完整实现日期: 2025-09-16
 * @author Claude Code Assistant
 */

import React from 'react';
import EndpointRow from './EndpointRow.jsx';

/**
 * 端点表格组件
 * @param {Object} props 组件属性
 * @param {Array} props.endpoints 端点数据数组，每个元素包含端点的完整信息
 * @param {boolean} props.loading 加载状态标识，为true时显示"加载中..."
 * @param {Function} props.onUpdatePriority 优先级更新回调函数 (endpointName, newPriority) => Promise
 * @param {Function} props.onHealthCheck 手动健康检测回调函数 (endpointName) => Promise
 * @returns {JSX.Element} 端点表格JSX元素
 */
const EndpointsTable = ({
    endpoints = [],
    loading = false,
    onUpdatePriority,
    onHealthCheck
}) => {
    // 加载状态：显示加载中信息
    if (loading) {
        return (
            <div className="endpoints-table">
                <p>加载中...</p>
            </div>
        );
    }

    // 空数据状态：显示暂无数据信息
    if (!endpoints || endpoints.length === 0) {
        return (
            <div className="endpoints-table">
                <p>暂无端点数据</p>
            </div>
        );
    }

    // 正常状态：显示完整的端点表格
    return (
        <div className="endpoints-table">
            <table>
                <thead>
                    <tr>
                        <th>状态</th>
                        <th>名称</th>
                        <th>URL</th>
                        <th>优先级</th>
                        <th>组</th>
                        <th>响应时间</th>
                        <th>最后检查</th>
                        <th>操作</th>
                    </tr>
                </thead>
                <tbody>
                    {endpoints.map((endpoint, index) => (
                        <EndpointRow
                            key={endpoint.name || `endpoint-${index}`}
                            endpoint={endpoint}
                            onUpdatePriority={onUpdatePriority}
                            onHealthCheck={onHealthCheck}
                        />
                    ))}
                </tbody>
            </table>
        </div>
    );
};

export default EndpointsTable;
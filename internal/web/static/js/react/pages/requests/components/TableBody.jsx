/**
 * TableBody - 表格主体组件
 * 文件描述: 渲染请求数据的表格主体内容，支持13列布局
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';
import RequestRow from './RequestRow.jsx';
import LoadingSpinner from './LoadingSpinner.jsx';

const TableBody = ({ requests, onRowClick, isInitialLoading }) => {
    // 处理行点击事件
    const handleRowClick = (request) => {
        if (onRowClick) {
            onRowClick(request);
        }
    };

    // 只在初次加载且没有数据时显示加载状态
    if (isInitialLoading) {
        return (
            <tbody>
                <tr>
                    <td colSpan="12" className="loading-cell">
                        <div className="loading-container">
                            <LoadingSpinner />
                            <span className="loading-text">正在加载请求数据...</span>
                        </div>
                    </td>
                </tr>
            </tbody>
        );
    }

    return (
        <tbody>
            {requests && requests.length > 0 ? (
                requests.map((request) => (
                    <RequestRow
                        key={request.requestId || request.id || Math.random()}
                        request={request}
                        onClick={() => handleRowClick(request)}
                    />
                ))
            ) : (
                <tr>
                    <td colSpan="12" className="no-data">
                        <div className="no-data-container">
                            <span className="no-data-icon">📝</span>
                            <span className="no-data-text">暂无请求数据</span>
                            <span className="no-data-hint">请发送一些请求后再查看</span>
                        </div>
                    </td>
                </tr>
            )}
        </tbody>
    );
};

export default TableBody;
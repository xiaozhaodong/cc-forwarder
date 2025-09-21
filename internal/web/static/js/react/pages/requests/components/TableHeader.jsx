/**
 * TableHeader - 表格头部组件
 * 文件描述: 定义请求表格的列头结构（12列标准布局）
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';

const TableHeader = () => {
    // 表头配置
    const columns = [
        { field: 'requestId', label: '请求ID' },
        { field: 'timestamp', label: '时间' },
        { field: 'status', label: '状态' },
        { field: 'model', label: '模型' },
        { field: 'endpoint', label: '端点' },
        { field: 'group', label: '组' },
        { field: 'duration', label: '耗时' },
        { field: 'inputTokens', label: '输入' },
        { field: 'outputTokens', label: '输出' },
        { field: 'cacheCreationTokens', label: '缓存创建' },
        { field: 'cacheReadTokens', label: '缓存读取' },
        { field: 'cost', label: '成本' }
    ];

    return (
        <thead>
            <tr>
                {columns.map((column) => (
                    <th key={column.field} className={`${column.field}-header`}>
                        <span className="column-label">{column.label}</span>
                    </th>
                ))}
            </tr>
        </thead>
    );
};

export default TableHeader;
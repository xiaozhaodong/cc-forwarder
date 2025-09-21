/**
 * LoadingSpinner - 加载动画组件
 * 文件描述: 显示数据加载过程中的转圈动画
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';

const LoadingSpinner = ({ size = 'medium', text = '加载中...' }) => {
    const sizeClass = `spinner-${size}`;

    return (
        <div className="loading-container">
            <div className={`loading-spinner ${sizeClass}`}>
                <div className="spinner"></div>
            </div>
            {text && (
                <div className="loading-text">
                    {text}
                </div>
            )}
        </div>
    );
};

export default LoadingSpinner;
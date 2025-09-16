/**
 * 操作按钮组件 (操作按钮)
 *
 * 负责：
 * - 提供端点相关的操作按钮(手动健康检测等)
 * - 处理操作按钮的点击事件和状态管理
 * - 与原版本endpointsManager.js完全兼容的交互逻辑
 * - 防止重复点击和错误处理
 * - 为未来更多操作按钮预留扩展空间
 *
 * 创建日期: 2025-09-15 23:47:50
 * 修改日期: 2025-09-16 00:05:00
 */

import React, { useState } from 'react';

const ActionButtons = ({
    endpoint,
    onHealthCheck,
    priorityEditorRef,
    disabled = false
}) => {
    // 按钮状态管理
    const [healthCheckLoading, setHealthCheckLoading] = useState(false);

    // 显示错误消息
    const showError = (message) => {
        // 使用全局Utils显示错误消息，与原版本保持一致
        if (window.Utils && window.Utils.showError) {
            window.Utils.showError(message);
        } else {
            console.error('ActionButtons错误:', message);
        }
    };

    // 显示成功消息
    const showSuccess = (message) => {
        // 使用全局Utils显示成功消息，与原版本保持一致
        if (window.Utils && window.Utils.showSuccess) {
            window.Utils.showSuccess(message);
        } else {
            console.log('ActionButtons成功:', message);
        }
    };

    // 更新优先级处理函数 - 通过ref调用PriorityEditor的方法
    const handleUpdatePriority = async () => {
        if (!priorityEditorRef || !priorityEditorRef.current) {
            showError('无法访问优先级编辑器');
            return;
        }

        try {
            // 调用PriorityEditor的executeUpdate方法
            await priorityEditorRef.current.executeUpdate();
        } catch (error) {
            console.error('更新优先级失败:', error);
            showError('更新优先级失败');
        }
    };
    const handleHealthCheck = async () => {
        if (!endpoint || !endpoint.name) {
            showError('端点信息无效');
            return;
        }

        if (!onHealthCheck) {
            showError('健康检测功能不可用');
            return;
        }

        try {
            setHealthCheckLoading(true);

            // 调用健康检测回调函数，传入端点名称
            const result = await onHealthCheck(endpoint.name);

            if (result && result.success === false) {
                // 检测失败，显示错误消息
                showError(result.error || '手动检测失败');
            } else if (result && typeof result.healthy === 'boolean') {
                // 检测成功，显示结果
                const healthText = result.healthy ? '健康' : '不健康';
                showSuccess(`手动检测完成 - ${endpoint.name}: ${healthText}`);
            } else {
                // 默认成功处理
                showSuccess(`手动检测完成 - ${endpoint.name}`);
            }

        } catch (error) {
            console.error('手动检测失败:', error);
            showError('手动检测失败');
        } finally {
            setHealthCheckLoading(false);
        }
    };

    return (
        <div className="action-buttons">
            {/* 更新优先级按钮 */}
            <button
                className="btn btn-sm update-priority"
                data-endpoint={endpoint.name}
                onClick={handleUpdatePriority}
                disabled={disabled || healthCheckLoading || (priorityEditorRef?.current?.isUpdating)}
                title="更新优先级"
            >
                {(priorityEditorRef?.current?.isUpdating) ? '更新中...' : '更新'}
            </button>

            {/* 手动健康检测按钮 */}
            <button
                className="btn btn-sm manual-health-check"
                data-endpoint={endpoint.name}
                onClick={handleHealthCheck}
                disabled={disabled || healthCheckLoading || (priorityEditorRef?.current?.isUpdating)}
                title="手动健康检测"
            >
                {healthCheckLoading ? '检测中...' : '检测'}
            </button>

            {/* 预留扩展空间：未来可以添加更多操作按钮 */}
            {/*
            <button
                className="btn btn-sm endpoint-toggle"
                data-endpoint={endpoint.name}
                disabled={disabled}
                title="启用/禁用端点"
            >
                {endpoint.enabled ? '禁用' : '启用'}
            </button>

            <button
                className="btn btn-sm endpoint-test"
                data-endpoint={endpoint.name}
                disabled={disabled}
                title="测试连接"
            >
                测试
            </button>
            */}
        </div>
    );
};

export default ActionButtons;
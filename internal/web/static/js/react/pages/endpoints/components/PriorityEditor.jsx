/**
 * 优先级编辑器组件 (优先级输入框 + 更新逻辑)
 *
 * 负责：
 * - 显示端点的当前优先级
 * - 管理优先级编辑状态和验证逻辑
 * - 提供更新方法给ActionButtons组件调用
 * - 支持回车键快速确认更新
 * - 与原版本endpointsManager.js完全兼容的交互逻辑
 *
 * 创建日期: 2025-09-15 23:47:50
 * 修改日期: 2025-09-16 09:36:00 - 重构为React最佳实践
 * @author Claude Code Assistant
 */

import React, { useState, useRef, useEffect, useImperativeHandle } from 'react';

const PriorityEditor = React.forwardRef(({ priority, endpointName, onUpdate }, ref) => {
    // 状态管理
    const [currentPriority, setCurrentPriority] = useState(priority);
    const [isUpdating, setIsUpdating] = useState(false);
    const inputRef = useRef(null);

    // 同步外部优先级变化
    useEffect(() => {
        setCurrentPriority(priority);
    }, [priority]);

    // 输入值变化处理
    const handleInputChange = (e) => {
        setCurrentPriority(e.target.value);
    };

    // 数据验证
    const validatePriority = (value) => {
        const numValue = parseInt(value);
        if (isNaN(numValue) || numValue < 1) {
            return '优先级必须大于等于1';
        }
        return null;
    };

    // 显示错误消息
    const showError = (message) => {
        if (window.Utils && window.Utils.showError) {
            window.Utils.showError(message);
        } else {
            console.error('PriorityEditor错误:', message);
        }
    };

    // 显示成功消息
    const showSuccess = (message) => {
        if (window.Utils && window.Utils.showSuccess) {
            window.Utils.showSuccess(message);
        } else {
            console.log('PriorityEditor成功:', message);
        }
    };

    // 执行更新操作 - 暴露给ActionButtons组件
    const executeUpdate = async () => {
        // 验证数据
        const validationError = validatePriority(currentPriority);
        if (validationError) {
            showError(validationError);
            return { success: false, error: validationError };
        }

        const newPriority = parseInt(currentPriority);

        // 如果值没有改变，不执行更新
        if (newPriority === priority) {
            showSuccess('优先级未改变');
            return { success: true, message: '优先级未改变' };
        }

        setIsUpdating(true);

        try {
            console.log('执行优先级更新:', endpointName, newPriority);

            // 调用父组件的更新方法
            const result = await onUpdate(endpointName, newPriority);

            if (result && result.success === false) {
                // 更新失败，显示错误消息并重置值
                showError(result.error || '更新失败');
                setCurrentPriority(priority);
                return result;
            } else {
                // 更新成功
                showSuccess('优先级更新成功');
                return { success: true, message: '优先级更新成功' };
            }

        } catch (error) {
            console.error('优先级更新失败:', error);
            showError('更新优先级失败');
            // 重置为原始值
            setCurrentPriority(priority);
            return { success: false, error: '更新优先级失败' };
        } finally {
            setIsUpdating(false);
        }
    };

    // 回车键处理 - 直接调用executeUpdate
    const handleKeyPress = (e) => {
        if (e.key === 'Enter') {
            executeUpdate();
        }
    };

    // 暴露方法给ActionButtons组件使用
    useImperativeHandle(ref, () => ({
        executeUpdate,
        isUpdating
    }));

    return (
        <input
            ref={inputRef}
            type="number"
            className="priority-input"
            value={currentPriority}
            onChange={handleInputChange}
            onKeyPress={handleKeyPress}
            min="1"
            disabled={isUpdating}
            data-endpoint={endpointName}
            style={{
                width: '60px',
                padding: '2px 6px',
                border: '1px solid #ddd',
                borderRadius: '3px'
            }}
        />
    );
});

PriorityEditor.displayName = 'PriorityEditor';

export default PriorityEditor;
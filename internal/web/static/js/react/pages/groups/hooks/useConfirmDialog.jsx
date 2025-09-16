/**
 * 确认对话框Hook - 提供确认对话框状态管理和交互控制
 *
 * 负责：
 * - 确认对话框的显示/隐藏状态管理
 * - 对话框配置管理（标题、消息、按钮等）
 * - Promise化的确认流程
 * - 支持多种对话框类型和样式
 * - 应急激活特殊支持
 *
 * 功能特性：
 * - Promise化确认流程
 * - 灵活的对话框配置
 * - 多种对话框类型支持（default, emergency）
 * - 应急激活详细信息显示
 * - 自动状态重置
 * - 键盘快捷键支持
 * - 焦点管理和可访问性
 *
 * API兼容性：
 * - showConfirmDialog - 符合迁移计划的新API
 * - DialogComponent - 标准渲染组件
 *
 * 创建日期: 2025-09-16
 * 更新日期: 2025-09-16 - 基于迁移计划重构API
 * @author Claude Code Assistant
 */

import { useState, useCallback, useRef, useEffect } from 'react';
import ConfirmDialog from '../components/ConfirmDialog.jsx';

/**
 * 确认对话框Hook
 * 基于迁移计划实现，支持新的API格式和应急激活功能
 *
 * @returns {Object} Hook返回对象
 */
const useConfirmDialog = () => {
    // 对话框状态 - 符合迁移计划的状态结构
    const [dialogState, setDialogState] = useState({
        isOpen: false,
        options: {},
        resolve: null
    });

    // 防止重复显示对话框
    const isShowingRef = useRef(false);

    // 显示确认对话框 - 符合迁移计划API
    const showConfirmDialog = useCallback((options = {}) => {
        // 防止重复显示
        if (isShowingRef.current) {
            console.warn('⚠️ [确认对话框] 已有对话框显示中，忽略新请求');
            return Promise.resolve(false);
        }

        return new Promise((resolve) => {
            isShowingRef.current = true;

            // 设置默认选项
            const dialogOptions = {
                title: '确认操作',
                message: '您确定要执行此操作吗？',
                confirmText: '确定',
                cancelText: '取消',
                icon: 'ℹ️',
                variant: 'default',
                ...options
            };

            // 根据variant设置类型映射
            const typeMapping = {
                'default': 'info',
                'emergency': 'danger'
            };

            // 为ConfirmDialog组件准备配置
            const confirmDialogConfig = {
                title: dialogOptions.title,
                message: dialogOptions.message,
                confirmText: dialogOptions.confirmText,
                cancelText: dialogOptions.cancelText,
                type: typeMapping[dialogOptions.variant] || 'info',
                details: dialogOptions.details,
                warning: dialogOptions.warning,
                confirmButtonStyle: dialogOptions.variant === 'emergency' ?
                    { backgroundColor: '#dc2626', color: 'white' } : {}
            };

            // 更新对话框状态
            setDialogState({
                isOpen: true,
                options: confirmDialogConfig,
                resolve
            });

            console.log('🔍 [确认对话框] 显示对话框:', {
                variant: dialogOptions.variant,
                title: dialogOptions.title,
                hasDetails: !!dialogOptions.details,
                hasWarning: !!dialogOptions.warning
            });
        });
    }, []);

    // 重置对话框状态
    const resetDialog = useCallback(() => {
        setDialogState({
            isOpen: false,
            options: {},
            resolve: null
        });
        isShowingRef.current = false;
        console.log('🔍 [确认对话框] 对话框状态已重置');
    }, []);

    // 处理确认操作
    const handleConfirm = useCallback(() => {
        console.log('✅ [确认对话框] 用户确认操作');

        if (dialogState.resolve) {
            dialogState.resolve(true);
        }

        resetDialog();
    }, [dialogState.resolve, resetDialog]);

    // 处理取消操作
    const handleCancel = useCallback(() => {
        console.log('❌ [确认对话框] 用户取消操作');

        if (dialogState.resolve) {
            dialogState.resolve(false);
        }

        resetDialog();
    }, [dialogState.resolve, resetDialog]);

    // 键盘事件处理和焦点管理
    useEffect(() => {
        if (!dialogState.isOpen) return;

        // 保存当前焦点元素，用于对话框关闭后恢复
        const previousActiveElement = document.activeElement;

        const handleKeyDown = (event) => {
            // 只在对话框打开时处理键盘事件
            if (!dialogState.isOpen) return;

            switch (event.key) {
                case 'Escape':
                    event.preventDefault();
                    event.stopPropagation();
                    handleCancel();
                    break;
                case 'Enter':
                    // 避免在输入框等元素中触发
                    if (event.target.tagName !== 'BUTTON' &&
                        event.target.tagName !== 'INPUT' &&
                        event.target.tagName !== 'TEXTAREA') {
                        event.preventDefault();
                        event.stopPropagation();
                        handleConfirm();
                    }
                    break;
                case 'Tab':
                    // Tab键焦点循环管理
                    const focusableElements = document.querySelectorAll(
                        '.confirm-dialog button:not([disabled]), ' +
                        '.confirm-dialog input:not([disabled]), ' +
                        '.confirm-dialog select:not([disabled]), ' +
                        '.confirm-dialog textarea:not([disabled]), ' +
                        '.confirm-dialog [tabindex]:not([tabindex="-1"])'
                    );

                    if (focusableElements.length > 0) {
                        const firstElement = focusableElements[0];
                        const lastElement = focusableElements[focusableElements.length - 1];

                        if (event.shiftKey) {
                            // Shift+Tab - 反向导航
                            if (document.activeElement === firstElement) {
                                event.preventDefault();
                                lastElement.focus();
                            }
                        } else {
                            // Tab - 正向导航
                            if (document.activeElement === lastElement) {
                                event.preventDefault();
                                firstElement.focus();
                            }
                        }
                    }
                    break;
            }
        };

        // 添加键盘事件监听器
        document.addEventListener('keydown', handleKeyDown, true);

        // 自动聚焦到第一个可聚焦元素
        const focusFirstElement = () => {
            const focusableElements = document.querySelectorAll(
                '.confirm-dialog button:not([disabled]), ' +
                '.confirm-dialog input:not([disabled]), ' +
                '.confirm-dialog select:not([disabled]), ' +
                '.confirm-dialog textarea:not([disabled]), ' +
                '.confirm-dialog [tabindex]:not([tabindex="-1"])'
            );

            if (focusableElements.length > 0) {
                // 优先聚焦到确认按钮
                const confirmButton = document.querySelector('.confirm-dialog-btn-confirm');
                if (confirmButton) {
                    confirmButton.focus();
                } else {
                    focusableElements[0].focus();
                }
            }
        };

        // 延迟聚焦，确保DOM已渲染
        const focusTimer = setTimeout(focusFirstElement, 0);

        // 清理函数
        return () => {
            document.removeEventListener('keydown', handleKeyDown, true);
            clearTimeout(focusTimer);

            // 恢复之前的焦点
            if (previousActiveElement && previousActiveElement.focus) {
                try {
                    previousActiveElement.focus();
                } catch (error) {
                    // 忽略焦点恢复错误
                    console.warn('无法恢复焦点:', error);
                }
            }
        };
    }, [dialogState.isOpen, handleConfirm, handleCancel]);

    // 组件卸载时的清理
    useEffect(() => {
        return () => {
            if (dialogState.resolve) {
                dialogState.resolve(false);
            }
            isShowingRef.current = false;
        };
    }, []);

    // 渲染确认对话框组件 - 符合迁移计划API
    const DialogComponent = useCallback(() => {
        return (
            <ConfirmDialog
                isOpen={dialogState.isOpen}
                config={dialogState.options}
                onConfirm={handleConfirm}
                onCancel={handleCancel}
            />
        );
    }, [dialogState.isOpen, dialogState.options, handleConfirm, handleCancel]);

    // 返回符合迁移计划的API接口
    return {
        showConfirmDialog,  // Promise化确认函数
        DialogComponent     // 渲染用的对话框组件
    };
};

export default useConfirmDialog;
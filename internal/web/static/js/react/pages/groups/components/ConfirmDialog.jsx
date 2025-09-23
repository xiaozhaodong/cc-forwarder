/**
 * 自定义确认对话框组件
 *
 * 负责：
 * - 提供用户操作确认界面
 * - 支持自定义确认消息和按钮
 * - 模态对话框样式和交互
 * - 操作结果反馈
 *
 * 功能特性：
 * - 灵活的对话框配置
 * - 多种对话框类型（info, warning, danger）
 * - 自定义按钮样式和文本
 * - 键盘快捷键支持（ESC取消，Enter确认）
 * - 模态遮罩和动画效果
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

import { useEffect, useRef } from 'react';

/**
 * 自定义确认对话框组件
 *
 * @param {Object} props - 组件属性
 * @param {boolean} props.isOpen - 对话框是否显示
 * @param {Object} props.config - 对话框配置对象
 * @param {string} props.config.title - 对话框标题
 * @param {string} props.config.message - 确认消息
 * @param {string} props.config.confirmText - 确认按钮文本
 * @param {string} props.config.cancelText - 取消按钮文本
 * @param {string} props.config.type - 对话框类型（info, warning, danger）
 * @param {Object} props.config.confirmButtonStyle - 确认按钮自定义样式
 * @param {Function} props.onConfirm - 确认回调
 * @param {Function} props.onCancel - 取消回调
 * @returns {JSX.Element} 确认对话框JSX元素
 */
const ConfirmDialog = ({
    isOpen = false,
    config = {},
    onConfirm = () => {},
    onCancel = () => {}
}) => {
    // 确认按钮引用 - 用于自动聚焦
    const confirmButtonRef = useRef(null);
    // 对话框配置默认值
    const dialogConfig = {
        title: '确认操作',
        message: '确定要执行此操作吗？',
        confirmText: '确认',
        cancelText: '取消',
        type: 'info',
        confirmButtonStyle: {},
        ...config
    };

    // 键盘事件处理和自动聚焦
    useEffect(() => {
        if (!isOpen) return;

        // 自动聚焦到确认按钮（与原版行为一致）
        if (confirmButtonRef.current) {
            confirmButtonRef.current.focus();
        }

        const handleKeyDown = (event) => {
            if (event.key === 'Escape') {
                onCancel();
            } else if (event.key === 'Enter') {
                onConfirm();
            }
        };

        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onConfirm, onCancel]);

    // 获取对话框类型样式
    const getTypeStyles = () => {
        switch (dialogConfig.type) {
            case 'danger':
                return {
                    iconColor: '#ef4444',
                    icon: '⚠️',
                    titleColor: '#dc2626',
                    borderColor: '#fecaca',
                    backgroundColor: '#fef2f2'
                };
            case 'warning':
                return {
                    iconColor: '#f59e0b',
                    icon: '⚠️',
                    titleColor: '#d97706',
                    borderColor: '#fde68a',
                    backgroundColor: '#fffbeb'
                };
            default: // info
                return {
                    iconColor: '#3b82f6',
                    icon: 'ℹ️',
                    titleColor: '#1d4ed8',
                    borderColor: '#dbeafe',
                    backgroundColor: '#eff6ff'
                };
        }
    };

    const typeStyles = getTypeStyles();

    // 处理遮罩点击关闭（与原版行为一致）
    const handleOverlayClick = (event) => {
        // 只有点击遮罩层本身才关闭，点击对话框内容不关闭
        if (event.target === event.currentTarget) {
            onCancel();
        }
    };

    // 如果对话框未打开，不渲染
    if (!isOpen) {
        return null;
    }

    return (
        <div
            className={`confirm-dialog-overlay show`}
            onClick={handleOverlayClick}
        >
            {/* 对话框容器 */}
            <div className="confirm-dialog">
                {/* 对话框头部 */}
                <div className="confirm-dialog-header">
                    <div className="confirm-dialog-icon">
                        {typeStyles.icon}
                    </div>
                    <h3 className="confirm-dialog-title">
                        {dialogConfig.title}
                    </h3>
                </div>

                {/* 对话框内容 */}
                <div className="confirm-dialog-body">
                    <div className="confirm-dialog-message">
                        {dialogConfig.message}
                    </div>

                    {/* 应急激活详情（如果提供） */}
                    {config.details && (
                        <div className="confirm-dialog-details">
                            <h4>应急激活将会:</h4>
                            <ul>
                                {config.details.map((item, index) => (
                                    <li key={index}>{item}</li>
                                ))}
                            </ul>
                        </div>
                    )}

                    {/* 警告信息（如果提供） */}
                    {config.warning && (
                        <div className="confirm-dialog-warning">
                            {config.warning}
                        </div>
                    )}
                </div>

                {/* 按钮区域 */}
                <div className="confirm-dialog-footer">
                    {/* 取消按钮 */}
                    <button
                        className="confirm-dialog-btn confirm-dialog-btn-cancel"
                        onClick={onCancel}
                    >
                        {dialogConfig.cancelText}
                    </button>

                    {/* 确认按钮 */}
                    <button
                        ref={confirmButtonRef}
                        className="confirm-dialog-btn confirm-dialog-btn-confirm"
                        style={dialogConfig.confirmButtonStyle}
                        onClick={onConfirm}
                    >
                        {dialogConfig.confirmText}
                    </button>
                </div>
            </div>
        </div>
    );
};

export default ConfirmDialog;
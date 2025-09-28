/**
 * RequestRow - 请求行组件
 * 文件描述: 显示单个请求记录的表格行（12列数据布局）
 * 创建时间: 2025-09-20 18:03:21
 */

import React, { useState, useRef } from 'react';
import StatusBadge from './StatusBadge.jsx';
import {
    formatDuration,
    formatTimestamp,
    formatRequestId,
    formatModelName,
    formatEndpoint,
    formatStreamingIcon,
    formatCost,
    getModelColorClass
} from '../utils/requestsFormatter.jsx';

const RequestRow = ({ request, onClick }) => {
    const [copyToast, setCopyToast] = useState(null);
    const doubleClickTimer = useRef(null);
    const clickCount = useRef(0);

    // 处理单元格点击（区分单击和双击）
    const handleCellClick = (value, fieldName, e) => {
        e.stopPropagation();
        clickCount.current++;

        // 保存当前元素引用，避免在setTimeout中丢失
        const targetElement = e.currentTarget;

        if (clickCount.current === 1) {
            // 单击 - 设置定时器等待可能的第二次点击
            doubleClickTimer.current = setTimeout(() => {
                // 确认是单击 - 执行复制
                // 创建一个新的事件对象，包含保存的元素引用
                const fakeEvent = {
                    currentTarget: targetElement
                };
                handleCopy(value, fieldName, fakeEvent);
                clickCount.current = 0;
            }, 250);
        } else if (clickCount.current === 2) {
            // 双击 - 清除定时器并执行详情查看
            clearTimeout(doubleClickTimer.current);
            clickCount.current = 0;
            if (onClick) {
                onClick(request);
            }
        }
    };

    // 处理复制操作
    const handleCopy = (value, fieldName, e) => {
        // 如果没有值或是空值，不执行复制
        if (!value || value === '-' || value === 'N/A' || value === '0') {
            return;
        }

        // 复制到剪贴板的函数
        const copyToClipboard = (text) => {
            // 优先使用 navigator.clipboard API
            if (navigator.clipboard && window.isSecureContext) {
                return navigator.clipboard.writeText(text);
            } else {
                // 降级方案：使用传统的 execCommand
                const textArea = document.createElement("textarea");
                textArea.value = text;
                textArea.style.position = "fixed";
                textArea.style.left = "-999999px";
                textArea.style.top = "-999999px";
                document.body.appendChild(textArea);
                textArea.focus();
                textArea.select();

                return new Promise((resolve, reject) => {
                    const result = document.execCommand('copy');
                    document.body.removeChild(textArea);
                    if (result) {
                        resolve();
                    } else {
                        reject(new Error('复制失败'));
                    }
                });
            }
        };

        // 执行复制
        copyToClipboard(value).then(() => {
            // 获取点击位置
            const rect = e.currentTarget.getBoundingClientRect();
            const x = rect.left + rect.width / 2;
            const y = rect.top;

            // 显示复制成功提示
            setCopyToast({
                x,
                y,
                text: `已复制: ${value}`,
                id: Date.now()
            });

            // 2秒后自动隐藏
            setTimeout(() => {
                setCopyToast(null);
            }, 2000);
        }).catch(err => {
            console.error('复制失败:', err);
            // 显示错误提示
            if (window.Utils && window.Utils.showError) {
                window.Utils.showError('复制失败，请手动选择文本复制');
            }
        });
    };

    // 格式化Token数量 - 直接显示原始数字，不做单位转换
    const formatTokenCount = (tokens) => {
        if (!tokens || tokens === 0) return '0';  // 零值显示 "0" 而不是 "N/A"
        const num = parseInt(tokens);
        if (isNaN(num)) return 'N/A';
        return num.toString();  // 直接返回原始数字，与原版保持一致
    };

    return (
        <>
            <tr className="request-row">
                {/* 1. 请求ID */}
                <td
                    className="request-id copyable"
                    onClick={(e) => handleCellClick(request.requestId, '请求ID', e)}
                >
                    <span className="id-text">
                        {formatStreamingIcon(request.isStreaming)}{' '}
                        {formatRequestId(request.requestId)}
                    </span>
                </td>

                {/* 2. 时间 */}
                <td
                    className="timestamp copyable"
                    onClick={(e) => handleCellClick(formatTimestamp(request.timestamp), '时间', e)}
                >
                    {formatTimestamp(request.timestamp)}
                </td>

                {/* 3. 状态 */}
                <td
                    className="status copyable"
                    onClick={(e) => handleCellClick(request.status, '状态', e)}
                >
                    <StatusBadge status={request.status} />
                </td>

                {/* 4. 模型 */}
                <td
                    className="model copyable"
                    onClick={(e) => handleCellClick(request.model, '模型', e)}
                >
                    <span className={`model-badge ${getModelColorClass(request.model)}`}>
                        {formatModelName(request.model)}
                    </span>
                </td>

                {/* 5. 端点 */}
                <td
                    className="endpoint copyable"
                    onClick={(e) => handleCellClick(request.endpoint, '端点', e)}
                >
                    <span className="endpoint-name">{request.endpoint}</span>
                </td>

                {/* 6. 组 */}
                <td
                    className="group copyable"
                    onClick={(e) => handleCellClick(request.group, '组', e)}
                >
                    <span className="group-name">{request.group}</span>
                </td>

                {/* 7. 耗时 */}
                <td
                    className="duration copyable"
                    onClick={(e) => handleCellClick(formatDuration(request.duration), '耗时', e)}
                >
                    {formatDuration(request.duration)}
                </td>

                {/* 8. 输入Tokens */}
                <td
                    className="input-tokens copyable"
                    onClick={(e) => handleCellClick(request.inputTokens, '输入Tokens', e)}
                >
                    {formatTokenCount(request.inputTokens)}
                </td>

                {/* 9. 输出Tokens */}
                <td
                    className="output-tokens copyable"
                    onClick={(e) => handleCellClick(request.outputTokens, '输出Tokens', e)}
                >
                    {formatTokenCount(request.outputTokens)}
                </td>

                {/* 10. 缓存创建Tokens */}
                <td
                    className="cache-creation-tokens copyable"
                    onClick={(e) => handleCellClick(request.cacheCreationTokens, '缓存创建Tokens', e)}
                >
                    {formatTokenCount(request.cacheCreationTokens)}
                </td>

                {/* 11. 缓存读取Tokens */}
                <td
                    className="cache-read-tokens copyable"
                    onClick={(e) => handleCellClick(request.cacheReadTokens, '缓存读取Tokens', e)}
                >
                    {formatTokenCount(request.cacheReadTokens)}
                </td>

                {/* 12. 成本 */}
                <td
                    className="cost copyable"
                    onClick={(e) => handleCellClick(formatCost(request.cost), '成本', e)}
                >
                    {formatCost(request.cost)}
                </td>
            </tr>

            {/* 复制成功提示 */}
            {copyToast && (
                <div
                    className="copy-toast"
                    style={{
                        position: 'fixed',
                        left: `${copyToast.x}px`,
                        top: `${copyToast.y - 40}px`,
                        transform: 'translateX(-50%)',
                        zIndex: 9999
                    }}
                    key={copyToast.id}
                >
                    <div className="copy-toast-content">
                        <svg className="copy-toast-icon" width="16" height="16" viewBox="0 0 16 16">
                            <circle cx="8" cy="8" r="8" fill="#10b981"/>
                            <path
                                d="M5 8l2 2 4-4"
                                stroke="white"
                                strokeWidth="2"
                                fill="none"
                                strokeLinecap="round"
                                strokeLinejoin="round"
                            />
                        </svg>
                        <span className="copy-toast-text">{copyToast.text}</span>
                    </div>
                </div>
            )}
        </>
    );
};

export default RequestRow;
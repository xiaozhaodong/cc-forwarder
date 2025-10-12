/**
 * RequestDetailModal - 请求详情模态框组件
 * 文件描述: 显示单个请求完整详细信息的弹窗
 * 创建时间: 2025-09-20 18:03:21
 */

import React from 'react';
import { formatTimestamp, formatDuration, formatRequestStatus } from '../utils/requestsFormatter.jsx';

const RequestDetailModal = ({ request, isOpen, onClose }) => {
    if (!isOpen || !request) {
        return null;
    }

    // 处理模态框关闭
    const handleClose = () => {
        onClose();
    };

    // 处理背景点击关闭
    const handleBackdropClick = (event) => {
        if (event.target === event.currentTarget) {
            handleClose();
        }
    };

    // 格式化JSON显示
    const formatJson = (data) => {
        if (!data) return 'N/A';
        if (typeof data === 'string') {
            try {
                return JSON.stringify(JSON.parse(data), null, 2);
            } catch {
                return data;
            }
        }
        return JSON.stringify(data, null, 2);
    };

    return (
        <div className="modal-backdrop" onClick={handleBackdropClick}>
            <div className="modal-content">
                <div className="modal-header">
                    <h3>请求详情</h3>
                    <span className="modal-close modal-close-x" onClick={handleClose}>
                        &times;
                    </span>
                </div>

                <div className="modal-body">
                    {/* 基本信息 */}
                    <div className="detail-section">
                        <h3>📋 基本信息</h3>
                        <div className="detail-grid">
                            <div className="detail-item">
                                <label>请求ID:</label>
                                <code className="detail-value">{request.requestId}</code>
                            </div>
                            <div className="detail-item">
                                <label>状态:</label>
                                <span className={`status-badge status-${request.status}`}>
                                    {formatRequestStatus(request.status)}
                                </span>
                            </div>
                            <div className="detail-item">
                                <label>开始时间:</label>
                                <span className="detail-value">{formatTimestamp(request.start_time || request.startTime || request.timestamp)}</span>
                            </div>
                            <div className="detail-item">
                                <label>结束时间:</label>
                                <span className="detail-value">{formatTimestamp(request.end_time || request.endTime)}</span>
                            </div>
                        </div>
                    </div>

                    {/* 网络信息 */}
                    <div className="detail-section">
                        <h3>🌐 网络信息</h3>
                        <div className="detail-grid">
                            <div className="detail-item">
                                <label>请求方法:</label>
                                <span className="detail-value method-badge">{request.method || 'POST'}</span>
                            </div>
                            <div className="detail-item">
                                <label>请求路径:</label>
                                <code className="detail-value request-path">{request.path || '/v1/messages'}</code>
                            </div>
                            <div className="detail-item">
                                <label>请求类型:</label>
                                <span className="detail-value" title={request.isStreaming ? '流式请求 - 实时响应' : '常规请求 - 完整响应'}>
                                    {request.isStreaming ? '🌊 流式请求' : '🔄 常规请求'}
                                </span>
                            </div>
                            <div className="detail-item">
                                <label>客户端IP:</label>
                                <span className="detail-value">{request.client_ip || request.clientIp || '-'}</span>
                            </div>
                            <div className="detail-item">
                                <label>用户代理:</label>
                                <span className="detail-value user-agent">{request.user_agent || request.userAgent || '-'}</span>
                            </div>
                            <div className="detail-item">
                                <label>HTTP状态码:</label>
                                <span className="detail-value">{request.http_status_code || request.httpStatusCode || '-'}</span>
                            </div>
                            <div className="detail-item">
                                <label>重试次数:</label>
                                <span className="detail-value">{request.retry_count || request.retryCount || 0}</span>
                            </div>
                        </div>
                    </div>

                    {/* 服务信息 */}
                    <div className="detail-section">
                        <h3>🚀 服务信息</h3>
                        <div className="detail-grid">
                            <div className="detail-item">
                                <label>模型:</label>
                                <span className="detail-value model-name">{request.model || '-'}</span>
                            </div>
                            <div className="detail-item">
                                <label>端点:</label>
                                <span className="detail-value">{request.endpoint || '-'}</span>
                            </div>
                            <div className="detail-item">
                                <label>组:</label>
                                <span className="detail-value">{request.group || '-'}</span>
                            </div>
                            <div className="detail-item">
                                <label>耗时:</label>
                                <span className="detail-value">{formatDuration(request.duration)}</span>
                            </div>
                        </div>
                    </div>

                    {/* Token & 成本信息 */}
                    <div className="detail-section">
                        <h3>🪙 Token & 成本</h3>
                        <div className="detail-grid">
                            <div className="detail-item">
                                <label>输入Token:</label>
                                <span className="detail-value token-count">{request.inputTokens || 0}</span>
                            </div>
                            <div className="detail-item">
                                <label>输出Token:</label>
                                <span className="detail-value token-count">{request.outputTokens || 0}</span>
                            </div>
                            <div className="detail-item">
                                <label>缓存创建Token:</label>
                                <span className="detail-value token-count">{request.cacheCreationTokens || 0}</span>
                            </div>
                            <div className="detail-item">
                                <label>缓存读取Token:</label>
                                <span className="detail-value token-count">{request.cacheReadTokens || 0}</span>
                            </div>
                            <div className="detail-item">
                                <label>总成本:</label>
                                <span className="detail-value cost-value">{request.cost ? `$${parseFloat(request.cost).toFixed(4)}` : '$0.0000'}</span>
                            </div>
                        </div>
                    </div>

                    {/* 错误信息 (v3.5.0状态机重构 - 基于状态和新字段显示) */}
                    {(['failed', 'error', 'cancelled', 'timeout'].includes(request.status) ||
                      request.failure_reason || request.cancel_reason ||
                      request.error || request.error_message || request.errorMessage) && (
                        <div className="detail-section">
                            <h3>❌ 错误信息</h3>
                            <div className="detail-grid">
                                {/* 失败原因 (新字段) */}
                                {request.failure_reason && (
                                    <div className="detail-item">
                                        <label>失败原因:</label>
                                        <span className="detail-value failure-reason">{request.failure_reason}</span>
                                    </div>
                                )}

                                {/* 取消原因 (新字段) */}
                                {request.cancel_reason && (
                                    <div className="detail-item">
                                        <label>取消原因:</label>
                                        <span className="detail-value cancel-reason">{request.cancel_reason}</span>
                                    </div>
                                )}

                                {/* 详细错误信息 (兼容新旧字段) */}
                                {(request.last_failure_reason || request.error || request.error_message || request.errorMessage) && (
                                    <div className="detail-item detail-full-width">
                                        <label>详细信息:</label>
                                        <div className="detail-value">
                                            {request.last_failure_reason || request.error || request.error_message || request.errorMessage}
                                        </div>
                                    </div>
                                )}
                            </div>
                        </div>
                    )}

                    {/* 请求详情 */}
                    {request.requestBody && (
                        <div className="detail-section">
                            <h3>📤 请求体</h3>
                            <pre className="code-block">
                                {formatJson(request.requestBody)}
                            </pre>
                        </div>
                    )}

                    {/* 响应详情 */}
                    {request.responseBody && (
                        <div className="detail-section">
                            <h3>📥 响应体</h3>
                            <pre className="code-block">
                                {formatJson(request.responseBody)}
                            </pre>
                        </div>
                    )}
                </div>

                <div className="modal-footer">
                    <button className="btn btn-secondary modal-close" onClick={handleClose}>
                        关闭
                    </button>
                </div>
            </div>
        </div>
    );
};

export default RequestDetailModal;


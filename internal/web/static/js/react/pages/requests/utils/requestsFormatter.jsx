/**
 * requestsFormatter - 数据格式化工具
 * 文件描述: 提供请求数据的格式化和显示工具函数
 * 创建时间: 2025-09-20 18:03:21
 *
 * 主要格式化功能:
 * - 时间格式化: 相对时间显示 (刚刚、几分钟前、具体时间)
 * - Token格式化: 智能数字显示 (K, M单位)
 * - 成本格式化: 美元货币格式
 * - 耗时格式化: 毫秒/秒/分钟显示
 * - 状态徽章颜色映射
 * - 流式请求图标显示
 * - 模型名称映射和显示
 * - 请求ID格式化
 * - 重试次数显示
 */

// 格式化持续时间
export const formatDuration = (duration) => {
    if (!duration || duration === 0) return '-';  // 零值或空值显示'-'，与原版保持一致

    // 如果是数字，假设是毫秒
    if (typeof duration === 'number') {
        if (duration < 1000) {
            return `${duration}ms`;
        } else if (duration < 60000) {
            return `${(duration / 1000).toFixed(2)}s`;
        } else {
            const minutes = Math.floor(duration / 60000);
            const seconds = ((duration % 60000) / 1000).toFixed(0);
            return `${minutes}m ${seconds}s`;
        }
    }

    // 如果是字符串，尝试解析
    if (typeof duration === 'string') {
        // 处理类似 "1.234s" 的格式
        if (duration.endsWith('s')) {
            const seconds = parseFloat(duration);
            if (!isNaN(seconds)) {
                if (seconds < 1) {
                    return `${(seconds * 1000).toFixed(0)}ms`;
                } else if (seconds < 60) {
                    return `${seconds.toFixed(2)}s`;
                } else {
                    const minutes = Math.floor(seconds / 60);
                    const remainingSeconds = (seconds % 60).toFixed(0);
                    return `${minutes}m ${remainingSeconds}s`;
                }
            }
        }

        // 处理类似 "1234ms" 的格式
        if (duration.endsWith('ms')) {
            const ms = parseInt(duration);
            if (!isNaN(ms)) {
                return formatDuration(ms);
            }
        }
    }

    return duration.toString();
};

// 格式化时间戳
export const formatTimestamp = (timestamp) => {
    if (!timestamp) return 'N/A';

    // 后端返回RFC3339格式：2025-09-25T10:07:54.994712+08:00
    // 直接使用浏览器的Date解析，它会正确处理时区信息
    const date = new Date(timestamp);
    if (isNaN(date.getTime())) return 'Invalid Date';

    // 显示本地时间格式：2025/9/25 10:07:54
    // 使用toLocaleString确保显示用户本地时区的时间
    const year = date.getFullYear();
    const month = date.getMonth() + 1;  // 不补零，按原版格式
    const day = date.getDate();         // 不补零，按原版格式
    const hours = String(date.getHours()).padStart(2, '0');
    const minutes = String(date.getMinutes()).padStart(2, '0');
    const seconds = String(date.getSeconds()).padStart(2, '0');

    return `${year}/${month}/${day} ${hours}:${minutes}:${seconds}`;
};

// 格式化Token信息 - 直接显示原始数字，不做单位转换
export const formatTokens = (inputTokens, outputTokens) => {
    const input = parseInt(inputTokens) || 0;
    const output = parseInt(outputTokens) || 0;
    const total = input + output;

    // 零值显示 "0" 而不是 "N/A"
    if (total === 0) return '0';

    // 直接返回原始数字，与原版保持一致
    if (input === 0 && output > 0) {
        return output.toString();
    }

    if (output === 0 && input > 0) {
        return input.toString();
    }

    return `${input}/${output}`;
};

// 格式化文件大小
export const formatFileSize = (bytes) => {
    if (!bytes || bytes === 0) return 'N/A';

    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(1024));

    return `${(bytes / Math.pow(1024, i)).toFixed(1)} ${sizes[i]}`;
};

// 格式化请求方法
export const formatMethod = (method) => {
    if (!method) return 'POST';
    return method.toUpperCase();
};

// 格式化状态码
export const formatStatusCode = (statusCode) => {
    if (!statusCode) return 'N/A';

    const code = parseInt(statusCode);
    if (isNaN(code)) return statusCode;

    // 直接返回状态码数字
    return code.toString();
};

// 格式化请求状态（详情页显示：原始值 + 中文描述）
export const formatRequestStatus = (status) => {
    if (!status) return 'N/A';

    // 状态中文描述映射
    const statusDescriptions = {
        'pending': '等待中',
        'forwarding': '转发中',
        'processing': '处理中',
        'completed': '已完成',
        'error': '请求错误',
        'retry': '重试中',
        'cancelled': '已取消',
        'timeout': '超时',
        'suspended': '挂起',
        'network_error': '网络错误',
        'auth_error': '认证错误',
        'rate_limited': '限流错误',
        'stream_error': '流式错误'
    };

    const description = statusDescriptions[status];
    return description ? `${status} (${description})` : status;
};

// 格式化端点名称
export const formatEndpoint = (endpoint, group) => {
    if (!endpoint || endpoint === 'unknown') {
        return '-';
    }

    let formatted = endpoint;
    if (group && group !== 'default') {
        formatted += ` (${group})`;
    }

    return formatted;
};

// 截断长文本
export const truncateText = (text, maxLength = 50) => {
    if (!text) return '';
    if (text.length <= maxLength) return text;
    return text.substring(0, maxLength) + '...';
};

// 格式化成本
export const formatCost = (cost) => {
    if (!cost || cost === 0) return '$0.00';

    const numCost = parseFloat(cost);
    if (isNaN(numCost)) return 'N/A';

    if (numCost < 0.01) {
        return `$${numCost.toFixed(4)}`;
    } else if (numCost < 1) {
        return `$${numCost.toFixed(3)}`;
    } else {
        return `$${numCost.toFixed(2)}`;
    }
};

// 格式化成功率
export const formatSuccessRate = (rate) => {
    if (rate === null || rate === undefined) return 'N/A';

    const numRate = parseFloat(rate);
    if (isNaN(numRate)) return 'N/A';

    return `${numRate.toFixed(1)}%`;
};

// 格式化请求ID
export const formatRequestId = (requestId) => {
    if (!requestId) return 'N/A';

    // 确保转换为字符串类型（处理数字ID的情况）
    const idStr = String(requestId);

    // 如果是短ID格式 (req-xxxxxxxx)，直接返回
    if (idStr.startsWith('req-') && idStr.length === 12) {
        return idStr;
    }

    // 如果是长ID，截取前8位
    if (idStr.length > 12) {
        return `${idStr.substring(0, 8)}...`;
    }

    return idStr;
};

// 格式化模型名称
export const formatModelName = (modelName) => {
    if (!modelName || modelName === 'unknown') return '-';

    // 直接返回原始模型名称，不进行映射转换
    return modelName;
};

// 获取模型颜色类名
export const getModelColorClass = (modelName) => {
    if (!modelName || modelName === 'unknown') return 'model-unknown';

    const lowerName = modelName.toLowerCase();

    // Claude Sonnet 4 系列 - 橙色
    if (lowerName.includes('sonnet-4') || lowerName.includes('claude-sonnet-4')) {
        return 'model-sonnet-4';
    }

    // Claude 3.5 Haiku 系列 - 绿色
    if (lowerName.includes('3-5-haiku') || lowerName.includes('haiku')) {
        return 'model-haiku';
    }

    // Claude 3.5 Sonnet 系列 - 蓝色
    if (lowerName.includes('3-5-sonnet') || (lowerName.includes('sonnet') && lowerName.includes('3.5'))) {
        return 'model-sonnet-3-5';
    }

    // Claude Opus 系列 - 紫色
    if (lowerName.includes('opus')) {
        return 'model-opus';
    }

    // 其他未知模型 - 灰色
    return 'model-unknown';
};

// 格式化流式请求图标
export const formatStreamingIcon = (isStreaming) => {
    return isStreaming ? '🌊' : '🔄';  // 流式请求显示🌊，非流式请求显示🔄，与原版保持一致
};

// 格式化重试次数
export const formatRetryCount = (retryCount) => {
    if (!retryCount || retryCount === 0) return '';
    return `重试${retryCount}次`;
};
// 配置数据格式化工具函数
// 复制原版utils.js中的generateConfigDisplay逻辑，确保100%兼容性

/**
 * 格式化配置值显示
 * 与原版Utils.generateConfigDisplay保持一致的格式化逻辑
 * @param {any} value - 配置值
 * @returns {string} - 格式化后的字符串
 */
export function formatConfigValue(value) {
    if (value === null || value === undefined) {
        return '';
    }

    // 如果是对象，使用JSON.stringify格式化
    if (typeof value === 'object') {
        return JSON.stringify(value, null, 2);
    }

    // 其他类型直接转换为字符串
    return String(value);
}

/**
 * 检查是否为有效的配置区块
 * 只处理object类型且非null的数据
 * @param {any} sectionData - 区块数据
 * @returns {boolean} - 是否为有效区块
 */
export function isValidConfigSection(sectionData) {
    return typeof sectionData === 'object' && sectionData !== null;
}

/**
 * 获取配置区块的所有键值对
 * @param {object} sectionData - 区块数据
 * @returns {Array} - 键值对数组
 */
export function getConfigItems(sectionData) {
    if (!isValidConfigSection(sectionData)) {
        return [];
    }

    return Object.keys(sectionData).map(key => ({
        key,
        value: sectionData[key]
    }));
}

/**
 * 过滤并格式化配置数据
 * 与原版generateConfigDisplay的逻辑完全一致
 * @param {object} config - 完整配置数据
 * @returns {Array} - 格式化后的配置区块数组
 */
export function formatConfigData(config) {
    if (!config || typeof config !== 'object') {
        return [];
    }

    const sections = [];

    Object.keys(config).forEach(sectionName => {
        const sectionData = config[sectionName];

        // 只处理object类型且非null的数据（与原版逻辑一致）
        if (isValidConfigSection(sectionData)) {
            sections.push({
                name: sectionName,
                data: sectionData,
                items: getConfigItems(sectionData)
            });
        }
    });

    return sections;
}
// 配置数据获取Hook
// 从/api/v1/config获取配置数据，处理loading和error状态

import React, { useState, useEffect, useCallback } from 'react';

const useConfigData = () => {
    const [configData, setConfigData] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);

    // 获取配置数据的函数
    const fetchConfigData = useCallback(async () => {
        try {
            setLoading(true);
            setError(null);

            const response = await fetch('/api/v1/config');

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }

            const data = await response.json();
            setConfigData(data);

        } catch (err) {
            console.error('配置数据获取失败:', err);
            setError(err.message || '配置数据获取失败');
            setConfigData(null);
        } finally {
            setLoading(false);
        }
    }, []);

    // 手动刷新函数
    const refetch = useCallback(() => {
        fetchConfigData();
    }, [fetchConfigData]);

    // 组件挂载时获取数据
    useEffect(() => {
        fetchConfigData();
    }, [fetchConfigData]);

    return {
        configData,
        loading,
        error,
        refetch
    };
};

export default useConfigData;
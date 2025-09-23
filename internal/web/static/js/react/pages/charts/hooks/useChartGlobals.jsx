import { useEffect } from 'react';
import { Chart as ChartJS } from 'chart.js';

// Chart.js 全局配置 Hook
// 精确复制自 internal/web/static/js/charts.js 的全局默认设置
export const useChartGlobals = () => {
    useEffect(() => {
        // 检查Chart.js是否可用
        if (typeof ChartJS === 'undefined' || window.chartLoadFailed) {
            console.warn('Chart.js不可用，图表功能将被禁用');
            return;
        }

        // Chart.js 默认配置 - 精确复制原始配置
        ChartJS.defaults.responsive = true;
        ChartJS.defaults.maintainAspectRatio = false;
        ChartJS.defaults.plugins.legend.display = true;
        ChartJS.defaults.font.family = '"Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif';
        ChartJS.defaults.font.size = 12;

        // 设置中文
        ChartJS.defaults.locale = 'zh-CN';

        console.log('📊 Chart.js 全局配置已设置');
    }, []);

    // 返回Chart.js可用性状态
    const isChartAvailable = () => {
        return typeof ChartJS !== 'undefined' && !window.chartLoadFailed;
    };

    // 显示加载状态的工具函数
    const showLoadingState = (selector = '.chart-loading') => {
        document.querySelectorAll(selector).forEach(loading => {
            loading.style.display = 'block';
            loading.textContent = '加载图表中...';
            loading.style.color = '#6b7280';
        });
    };

    // 隐藏加载状态的工具函数
    const hideLoadingState = (selector = '.chart-loading') => {
        document.querySelectorAll(selector).forEach(loading => {
            loading.style.display = 'none';
        });
    };

    // 显示错误状态的工具函数
    const showErrorState = (message, selector = '.chart-loading') => {
        document.querySelectorAll(selector).forEach(loading => {
            loading.style.display = 'block';
            loading.textContent = message;
            loading.style.color = '#ef4444';
        });
    };

    // 显示图表禁用消息的工具函数
    const showChartDisabledMessage = (selector = '.chart-loading') => {
        document.querySelectorAll(selector).forEach(loading => {
            loading.style.display = 'block';
            loading.textContent = '图表功能暂不可用 (Chart.js加载失败)';
            loading.style.color = '#6b7280';
        });
    };

    return {
        isChartAvailable,
        showLoadingState,
        hideLoadingState,
        showErrorState,
        showChartDisabledMessage
    };
};
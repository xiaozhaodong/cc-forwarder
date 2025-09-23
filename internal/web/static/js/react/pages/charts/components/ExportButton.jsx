import React from 'react';
import useChartExport from '../hooks/useChartExport.jsx';

/**
 * 图表导出按钮组件
 * 精确复制现有HTML模板中的导出按钮样式和功能
 */
const ExportButton = ({
    chartName,
    filename,
    chartInstance = null,
    title = "导出图片",
    icon = "📷",
    disabled = false,
    onClick,
    className = "",
    ...props
}) => {
    const { exportChart } = useChartExport();

    const handleClick = () => {
        // 如果提供了自定义onClick处理器，先执行它
        if (onClick) {
            const result = onClick();
            // 如果自定义处理器返回false，则不执行默认导出
            if (result === false) {
                return;
            }
        }

        // 执行图表导出
        exportChart(chartName, filename, chartInstance);
    };

    return (
        <button
            className={className}
            onClick={handleClick}
            title={title}
            disabled={disabled}
            {...props}
        >
            {icon}
        </button>
    );
};

export default ExportButton;
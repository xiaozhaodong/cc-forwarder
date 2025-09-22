import React from 'react';

/**
 * 时间范围选择器组件
 * 精确复制现有HTML模板中的时间选择器样式和功能
 */
const TimeRangeSelector = ({
    value,
    onChange,
    chartType = 'default',
    options = []
}) => {
    // 默认时间选项配置 - 与HTML模板保持一致
    const getDefaultOptions = (chartType) => {
        switch (chartType) {
            case 'connectionActivity':
                return [
                    { value: 30, label: '30分钟' },
                    { value: 60, label: '1小时', selected: true },
                    { value: 180, label: '3小时' },
                    { value: 360, label: '6小时' }
                ];
            case 'requestTrend':
            case 'responseTime':
            case 'suspendedTrend':
            default:
                return [
                    { value: 15, label: '15分钟' },
                    { value: 30, label: '30分钟', selected: true },
                    { value: 60, label: '1小时' },
                    { value: 180, label: '3小时' }
                ];
        }
    };

    // 使用传入的options或默认options
    const timeOptions = options.length > 0 ? options : getDefaultOptions(chartType);

    // 确定当前选中值
    const currentValue = value || timeOptions.find(opt => opt.selected)?.value || timeOptions[0].value;

    // 处理选择变化
    const handleChange = (event) => {
        const newValue = parseInt(event.target.value);
        if (onChange) {
            onChange(newValue);
        }
    };

    return (
        <select
            value={currentValue}
            onChange={handleChange}
        >
            {timeOptions.map((option) => (
                <option
                    key={option.value}
                    value={option.value}
                >
                    {option.label}
                </option>
            ))}
        </select>
    );
};

export default TimeRangeSelector;
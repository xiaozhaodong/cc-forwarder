// 配置值展示组件
// 处理配置值的格式化显示，应用config-value CSS类

import { formatConfigValue } from '../utils/configFormatter.jsx';

const ConfigValue = ({ value }) => {
    const formattedValue = formatConfigValue(value);

    return (
        <span className="config-value">
            {formattedValue}
        </span>
    );
};

export default ConfigValue;
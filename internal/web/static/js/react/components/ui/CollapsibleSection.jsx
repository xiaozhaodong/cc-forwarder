// 可折叠区域组件 - 使用原始HTML结构和CSS类名
// 2025-09-15 16:45:37

import React from 'react';

const CollapsibleSection = ({ id, title, children, defaultExpanded = false }) => {
    const [expanded, setExpanded] = React.useState(defaultExpanded);

    React.useEffect(() => {
        // 同步展开状态到LocalStorage，与原始版本保持一致
        localStorage.setItem(`section-${id}`, expanded ? 'expanded' : 'collapsed');
    }, [expanded, id]);

    React.useEffect(() => {
        // 从LocalStorage恢复状态
        const savedState = localStorage.getItem(`section-${id}`);
        if (savedState === 'expanded') {
            setExpanded(true);
        } else if (savedState === 'collapsed') {
            setExpanded(false);
        }
    }, [id]);

    return (
        <div id={`${id}-section`} className="collapsible-section">
            <div
                className="section-header"
                onClick={() => setExpanded(!expanded)}
                style={{
                    cursor: 'pointer',
                    userSelect: 'none',
                    outline: 'none',
                    WebkitTapHighlightColor: 'transparent'
                }}
                onMouseDown={(e) => e.preventDefault()} // 防止选中效果
            >
                <h3>{title}</h3>
                <span
                    id={`${id}-indicator`}
                    className="collapse-indicator"
                    style={{
                        transform: expanded ? 'rotate(180deg)' : 'rotate(0deg)',
                        transition: 'transform 0.3s ease'
                    }}
                >
                    {expanded ? '▲' : '▼'}
                </span>
            </div>
            <div
                id={`${id}-content`}
                className={`section-content ${expanded ? 'expanded' : 'collapsed'}`}
            >
                {children}
            </div>
        </div>
    );
};

export default CollapsibleSection;
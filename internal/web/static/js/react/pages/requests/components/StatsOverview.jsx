/**
 * StatsOverview - 统计概览组件
 * 文件描述: 显示请求统计概览卡片（总请求数、成功率、平均耗时、总成本、总Token数、挂起数）
 * 创建时间: 2025-09-20 20:23:55
 */

const StatsOverview = ({ stats, isLoading, isRefreshing = false }) => {
    const statsCards = [
        {
            id: 'total-requests',
            icon: '🚀',
            value: stats?.totalRequests || 0,
            label: '总请求数',
            className: ''
        },
        {
            id: 'success-rate',
            icon: '✅',
            value: stats?.successRate || '-%',
            label: '成功率',
            className: 'success'
        },
        {
            id: 'avg-duration',
            icon: '⏱️',
            value: stats?.avgDuration || '-',
            label: '平均耗时',
            className: 'warning'
        },
        {
            id: 'total-cost',
            icon: '💰',
            value: stats?.totalCost || '$0.00',
            label: '总成本',
            className: 'info'
        },
        {
            id: 'total-tokens',
            icon: '🔤',
            value: stats?.totalTokens || '0.00M',
            label: '总Token数 (M)',
            className: 'primary'
        },
        {
            id: 'failed-requests',
            icon: '❌',
            value: stats?.failedRequests || 0,
            label: '失败请求数',
            className: 'error'
        }
    ];

    // 只在真正的初次加载时显示骨架屏
    if (isLoading) {
        return (
            <div className="stats-overview">
                {statsCards.map((card) => (
                    <div key={card.id} className={`stats-card ${card.className}`}>
                        <div className="stat-icon">{card.icon}</div>
                        <div className="stat-content">
                            <div className="stat-value">-</div>
                            <div className="stat-label">{card.label}</div>
                        </div>
                    </div>
                ))}
            </div>
        );
    }

    return (
        <div className={`stats-overview ${isRefreshing ? 'refreshing' : ''}`}>
            {statsCards.map((card) => (
                <div key={card.id} className={`stats-card ${card.className}`}>
                    <div className="stat-icon">{card.icon}</div>
                    <div className="stat-content">
                        <div className="stat-value" id={`${card.id}-count`}>
                            {card.value}
                        </div>
                        <div className="stat-label">{card.label}</div>
                    </div>
                    {/* 可选：在刷新时显示一个小的角标指示器，但为了完全静默，我们暂时注释掉 */}
                    {/*isRefreshing && (
                        <div className="refresh-badge">
                            <div className="refresh-dot"></div>
                        </div>
                    )*/}
                </div>
            ))}
        </div>
    );
};

export default StatsOverview;
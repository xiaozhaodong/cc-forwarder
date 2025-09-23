/**
 * 组管理页面主组件 - 极简版本，完全匹配原版
 *
 * 只保留原版功能：
 * - 简单的标题
 * - 挂起请求警告
 * - 组统计概要（3个指标）
 * - 组卡片列表
 * - 应急激活确认对话框
 *
 * 移除的功能（原版没有）：
 * - 刷新按钮
 * - 详情模态框
 * - 额外的组件和功能
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

import useGroupsData from './hooks/useGroupsData.jsx';
import useGroupActions from './hooks/useGroupActions.jsx';
import useConfirmDialog from './hooks/useConfirmDialog.jsx';
import SuspendedAlert from './components/SuspendedAlert.jsx';
import GroupsSummary from './components/GroupsSummary.jsx';
import GroupsGrid from './components/GroupsGrid.jsx';

/**
 * 组管理页面主组件
 * 完全匹配原版templates.go的HTML结构
 * @returns {JSX.Element} 组管理页面JSX元素
 */
const GroupsPage = () => {
    // 获取组数据
    const {
        groups: data,
        loading,
        error,
        refreshGroups
    } = useGroupsData();

    // 确认对话框
    const { showConfirmDialog, DialogComponent } = useConfirmDialog();

    // 获取组操作方法（传入依赖函数避免重复状态）
    const {
        activateGroup,
        pauseGroup,
        resumeGroup,
        forceActivateGroup
    } = useGroupActions(refreshGroups, showConfirmDialog);

    return (
        <div className="section">
            {/* 页面标题 */}
            <h2>📦 组管理</h2>

            {/* 挂起请求警告横幅 */}
            {data && data.total_suspended_requests > 0 && (
                <SuspendedAlert
                    totalSuspendedRequests={data.total_suspended_requests}
                    groupSuspendedCounts={data.group_suspended_counts || {}}
                />
            )}

            {/* 组卡片网格 */}
            <GroupsGrid
                groups={data?.groups || []}
                loading={loading}
                error={error}
                onActivate={activateGroup}
                onPause={pauseGroup}
                onResume={resumeGroup}
                onForceActivate={forceActivateGroup}
            />

            {/* 组统计概要 */}
            <div className="groups-container" id="groups-container">
                <GroupsSummary data={data} />
            </div>

            {/* 确认对话框 */}
            <DialogComponent />
        </div>
    );
};

export default GroupsPage;
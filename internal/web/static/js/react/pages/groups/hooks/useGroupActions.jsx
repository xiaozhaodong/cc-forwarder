/**
 * 组操作Hook - 极简版本，完全匹配原版
 *
 * 原版行为：
 * - 激活、暂停、恢复：直接调用API，无确认对话框
 * - 应急激活：需要确认对话框
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

/**
 * 组操作Hook
 * 完全匹配原版groupsManager.js的操作行为
 *
 * @param {Function} refreshGroups - 数据刷新函数（由页面传入）
 * @param {Function} showConfirmDialog - 确认对话框函数（由页面传入）
 */
const useGroupActions = (refreshGroups, showConfirmDialog) => {
    // 直接激活组 - 无确认对话框
    const activateGroup = async (groupName) => {
        try {
            const response = await fetch(`/api/v1/groups/${groupName}/activate`, {
                method: 'POST'
            });
            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(errorData.error || '激活组失败');
            }
            const result = await response.json();
            Utils.showSuccess(result.message || `组 ${groupName} 已激活`);

            // 刷新数据（使用页面传入的函数）
            if (refreshGroups) {
                refreshGroups();
            }
            return true;
        } catch (error) {
            Utils.showError('激活组失败: ' + error.message);
            return false;
        }
    };

    // 直接暂停组 - 无确认对话框
    const pauseGroup = async (groupName) => {
        try {
            const response = await fetch(`/api/v1/groups/${groupName}/pause`, {
                method: 'POST'
            });
            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(errorData.error || '暂停组失败');
            }
            const result = await response.json();
            Utils.showSuccess(result.message || `组 ${groupName} 已暂停`);

            // 刷新数据
            if (refreshGroups) {
                refreshGroups();
            }
            return true;
        } catch (error) {
            Utils.showError('暂停组失败: ' + error.message);
            return false;
        }
    };

    // 直接恢复组 - 无确认对话框
    const resumeGroup = async (groupName) => {
        try {
            const response = await fetch(`/api/v1/groups/${groupName}/resume`, {
                method: 'POST'
            });
            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(errorData.error || '恢复组失败');
            }
            const result = await response.json();
            Utils.showSuccess(result.message || `组 ${groupName} 已恢复`);

            // 刷新数据
            if (refreshGroups) {
                refreshGroups();
            }
            return true;
        } catch (error) {
            Utils.showError('恢复组失败: ' + error.message);
            return false;
        }
    };

    // 应急激活组 - 需要确认对话框
    const forceActivateGroup = async (groupName) => {
        if (!showConfirmDialog) {
            Utils.showError('确认对话框不可用');
            return false;
        }

        const confirmed = await showConfirmDialog({
            title: '应急激活警告',
            icon: '⚡',
            message: `您正在尝试强制激活组 "${groupName}"。`,
            details: [
                '立即激活目标组',
                '绕过冷却时间限制',
                '强制停用其他活跃组',
                '可能导致服务不稳定'
            ],
            warning: '这是紧急情况的最后手段，只有在确实需要时才使用。',
            confirmText: '确认应急激活',
            cancelText: '取消操作',
            variant: 'emergency'
        });

        if (confirmed) {
            try {
                const response = await fetch(`/api/v1/groups/${groupName}/activate?force=true`, {
                    method: 'POST'
                });
                if (!response.ok) {
                    const errorData = await response.json();
                    throw new Error(errorData.error || '应急激活失败');
                }
                const result = await response.json();
                Utils.showWarning(result.message || `组 ${groupName} 已应急激活`);

                // 刷新数据
                if (refreshGroups) {
                    refreshGroups();
                }
                return true;
            } catch (error) {
                Utils.showError('应急激活失败: ' + error.message);
                return false;
            }
        }
        return false;
    };

    return {
        activateGroup,
        pauseGroup,
        resumeGroup,
        forceActivateGroup
    };
};

export default useGroupActions;
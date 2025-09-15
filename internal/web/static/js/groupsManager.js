// Claude Request Forwarder - 组管理模块
// 处理组激活、暂停、恢复操作和挂起请求管理

// 自定义确认对话框类
class ConfirmDialog {
    constructor() {
        this.isOpen = false;
        this.overlay = null;
    }

    // 显示确认对话框
    show(options = {}) {
        return new Promise((resolve) => {
            if (this.isOpen) {
                resolve(false);
                return;
            }

            this.isOpen = true;
            this.createDialog(options, resolve);
        });
    }

    // 创建对话框DOM结构
    createDialog(options, resolve) {
        const {
            title = '确认操作',
            message = '您确定要执行此操作吗？',
            confirmText = '确定',
            cancelText = '取消',
            icon = '⚠️',
            details = null,
            warning = null,
            groupName = ''
        } = options;

        // 创建遮罩层
        this.overlay = document.createElement('div');
        this.overlay.className = 'confirm-dialog-overlay';

        // 创建对话框
        const dialog = document.createElement('div');
        dialog.className = 'confirm-dialog';

        // 构建详细信息HTML
        let detailsHtml = '';
        if (details && Array.isArray(details)) {
            detailsHtml = `
                <div class="confirm-dialog-details">
                    <h4>应急激活将会:</h4>
                    <ul>
                        ${details.map(item => `<li>${item}</li>`).join('')}
                    </ul>
                </div>
            `;
        }

        // 构建警告信息HTML
        let warningHtml = '';
        if (warning) {
            warningHtml = `
                <div class="confirm-dialog-warning">
                    ${warning}
                </div>
            `;
        }

        dialog.innerHTML = `
            <div class="confirm-dialog-header">
                <div class="confirm-dialog-icon">${icon}</div>
                <h3 class="confirm-dialog-title">${title}</h3>
            </div>
            <div class="confirm-dialog-body">
                <div class="confirm-dialog-message">${message}</div>
                ${detailsHtml}
                ${warningHtml}
            </div>
            <div class="confirm-dialog-footer">
                <button class="confirm-dialog-btn confirm-dialog-btn-cancel" type="button">
                    ${cancelText}
                </button>
                <button class="confirm-dialog-btn confirm-dialog-btn-confirm" type="button">
                    ${confirmText}
                </button>
            </div>
        `;

        this.overlay.appendChild(dialog);
        document.body.appendChild(this.overlay);

        // 获取按钮元素
        const cancelBtn = dialog.querySelector('.confirm-dialog-btn-cancel');
        const confirmBtn = dialog.querySelector('.confirm-dialog-btn-confirm');

        // 绑定事件处理器
        const handleCancel = () => {
            this.close();
            resolve(false);
        };

        const handleConfirm = () => {
            this.close();
            resolve(true);
        };

        const handleKeyDown = (e) => {
            if (e.key === 'Escape') {
                handleCancel();
            } else if (e.key === 'Enter') {
                handleConfirm();
            }
        };

        const handleOverlayClick = (e) => {
            if (e.target === this.overlay) {
                handleCancel();
            }
        };

        // 添加事件监听器
        cancelBtn.addEventListener('click', handleCancel);
        confirmBtn.addEventListener('click', handleConfirm);
        document.addEventListener('keydown', handleKeyDown);
        this.overlay.addEventListener('click', handleOverlayClick);

        // 保存事件处理器以便清理
        this.cleanup = () => {
            document.removeEventListener('keydown', handleKeyDown);
            this.overlay.removeEventListener('click', handleOverlayClick);
        };

        // 显示动画
        requestAnimationFrame(() => {
            this.overlay.classList.add('show');
            confirmBtn.focus();
        });
    }

    // 关闭对话框
    close() {
        if (!this.isOpen || !this.overlay) {
            return;
        }

        this.isOpen = false;

        // 移除show类触发退出动画
        this.overlay.classList.remove('show');

        // 等待动画完成后移除DOM
        setTimeout(() => {
            if (this.overlay && this.overlay.parentNode) {
                this.overlay.parentNode.removeChild(this.overlay);
            }
            if (this.cleanup) {
                this.cleanup();
            }
            this.overlay = null;
            this.cleanup = null;
        }, 300);
    }

    // 静态方法：显示应急激活确认对话框
    static showForceActivationConfirm(groupName) {
        const dialog = new ConfirmDialog();
        return dialog.show({
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
            groupName
        });
    }
}

window.GroupsManager = class {
    constructor(webInterface) {
        this.webInterface = webInterface;
    }
    
    // 加载组信息
    async loadGroups() {
        try {
            const response = await fetch('/api/v1/groups');
            if (!response.ok) {
                throw new Error('获取组信息失败');
            }
            const data = await response.json();
            this.displayGroups(data);
            
            // 更新缓存
            this.webInterface.cachedData.groups = data;
            
        } catch (error) {
            console.error('加载组信息失败:', error);
            Utils.updateElementHTML('groups-container', 
                '<div class="error">❌ 加载组信息失败: ' + error.message + '</div>');
        }
    }
    
    // 显示组信息
    displayGroups(data) {
        // 显示组信息概要卡片
        const groupInfoCards = document.getElementById('group-info-cards');
        if (data.groups && data.groups.length > 0) {
            groupInfoCards.innerHTML = data.groups.map(group => this.createGroupCard(group)).join('');
        } else {
            groupInfoCards.innerHTML = '<div class="info">📦 没有配置的组</div>';
        }

        // 显示组统计概要
        const groupsContainer = document.getElementById('groups-container');
        const summaryHtml = `
            <div class="groups-summary">
                <div class="summary-item">
                    <div class="summary-value">${data.total_groups || 0}</div>
                    <div class="summary-label">总组数</div>
                </div>
                <div class="summary-item">
                    <div class="summary-value">${data.active_groups || 0}</div>
                    <div class="summary-label">活跃组数</div>
                </div>
                <div class="summary-item">
                    <div class="summary-value">${data.groups ? data.groups.reduce((sum, g) => sum + g.healthy_endpoints, 0) : 0}</div>
                    <div class="summary-label">健康端点</div>
                </div>
            </div>
        `;
        groupsContainer.innerHTML = summaryHtml;
        
        // 更新挂起请求提示
        this.updateGroupSuspendedAlert(data);
    }
    
    // 创建组信息卡片
    createGroupCard(group) {
        const statusClass = group.in_cooldown ? 'cooldown' : (group.is_active ? 'active' : 'inactive');
        const statusText = group._computed_health_status || group.status || (group.in_cooldown ? '冷却中' : (group.is_active ? '活跃' : '未激活'));

        const cooldownInfo = group.in_cooldown && group.cooldown_remaining !== '0s' ?
            `<div class="group-cooldown-info">🕐 冷却剩余时间: ${group.cooldown_remaining}</div>` : '';

        // 应急激活状态显示
        const forceActivationInfo = group.is_force_activated ?
            `<div class="group-force-activation-info">⚡ 应急激活 - ${group.force_activation_time || '时间未知'}</div>` : '';

        return `
            <div class="group-info-card ${statusClass}" data-group-name="${group.name}">
                <div class="group-card-header">
                    <h3 class="group-name">${group.name}</h3>
                    <span class="group-status ${statusClass}">${statusText}${group.is_force_activated ? ' ⚡' : ''}</span>
                </div>
                <div class="group-details">
                    <div class="group-detail-item">
                        <div class="group-detail-label">优先级</div>
                        <div class="group-detail-value group-priority">${group.priority}</div>
                    </div>
                    <div class="group-detail-item">
                        <div class="group-detail-label">端点总数</div>
                        <div class="group-detail-value">${group.total_endpoints}</div>
                    </div>
                    <div class="group-detail-item">
                        <div class="group-detail-label">健康端点</div>
                        <div class="group-detail-value group-endpoints-count">${group.healthy_endpoints}</div>
                    </div>
                    <div class="group-detail-item">
                        <div class="group-detail-label">不健康端点</div>
                        <div class="group-detail-value group-unhealthy-count">${group.unhealthy_endpoints}</div>
                    </div>
                </div>
                <div class="group-actions">
                    <button class="group-btn btn-activate"
                            onclick="webInterface.groupsManager.activateGroup('${group.name}')"
                            ${!group.can_activate ? 'disabled' : ''}>
                        🚀 激活
                    </button>
                    ${group.can_force_activate ? `
                    <button class="group-btn btn-danger"
                            onclick="webInterface.groupsManager.forceActivateGroup('${group.name}')">
                        ⚡应急
                    </button>
                    ` : ''}
                    <button class="group-btn btn-pause"
                            onclick="webInterface.groupsManager.pauseGroup('${group.name}')"
                            ${!group.can_pause ? 'disabled' : ''}>
                        ⏸️ 暂停
                    </button>
                    <button class="group-btn btn-resume"
                            onclick="webInterface.groupsManager.resumeGroup('${group.name}')"
                            ${!group.can_resume ? 'disabled' : ''}>
                        ▶️ 恢复
                    </button>
                </div>
                ${cooldownInfo}
                ${forceActivationInfo}
            </div>
        `;
    }
    
    // 激活组
    async activateGroup(groupName, force = false) {
        try {
            const url = force ?
                `/api/v1/groups/${groupName}/activate?force=true` :
                `/api/v1/groups/${groupName}/activate`;

            const response = await fetch(url, {
                method: 'POST'
            });
            if (!response.ok) {
                const errorData = await response.json();
                throw new Error(errorData.error || '激活组失败');
            }
            const result = await response.json();

            if (force) {
                Utils.showWarning(result.message || `组 ${groupName} 已应急激活`);
            } else {
                Utils.showSuccess(result.message || `组 ${groupName} 已激活`);
            }

            // 刷新组数据
            this.loadGroups();
        } catch (error) {
            console.error('激活组失败:', error);
            Utils.showError('激活组失败: ' + error.message);
        }
    }
    
    // 暂停组
    async pauseGroup(groupName) {
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
            
            // 刷新组数据
            this.loadGroups();
        } catch (error) {
            console.error('暂停组失败:', error);
            Utils.showError('暂停组失败: ' + error.message);
        }
    }
    
    // 恢复组
    async resumeGroup(groupName) {
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

            // 刷新组数据
            this.loadGroups();
        } catch (error) {
            console.error('恢复组失败:', error);
            Utils.showError('恢复组失败: ' + error.message);
        }
    }

    // 应急激活组（强制激活）
    async forceActivateGroup(groupName) {
        try {
            // 使用自定义确认对话框替代原生confirm
            const confirmed = await ConfirmDialog.showForceActivationConfirm(groupName);

            if (!confirmed) {
                return;
            }

            await this.activateGroup(groupName, true);
        } catch (error) {
            console.error('应急激活失败:', error);
            Utils.showError('应急激活失败: ' + error.message);
        }
    }
    
    // 更新组管理界面的挂起提示
    updateGroupSuspendedAlert(groupData) {
        const alertBanner = document.getElementById('group-suspended-alert');
        const alertMessage = document.getElementById('suspended-alert-message');
        
        if (!alertBanner || !alertMessage) return;

        const totalSuspended = groupData.total_suspended_requests || 0;
        const groupCounts = groupData.group_suspended_counts || {};

        if (totalSuspended > 0) {
            let message = `当前有 ${totalSuspended} 个挂起请求等待处理`;
            const suspendedGroups = Object.entries(groupCounts)
                .filter(([group, count]) => count > 0)
                .map(([group, count]) => `${group}(${count})`)
                .join(', ');
            
            if (suspendedGroups) {
                message += `，涉及组: ${suspendedGroups}`;
            }
            
            alertMessage.textContent = message;
            alertBanner.style.display = 'flex';
        } else {
            alertBanner.style.display = 'none';
        }
    }
    
    // 隐藏挂起请求警告
    hideSuspendedAlert() {
        const alertBanner = document.getElementById('group-suspended-alert');
        if (alertBanner) {
            alertBanner.style.display = 'none';
        }
    }
    
    // 获取组状态概览
    getGroupsOverview() {
        const cachedData = this.webInterface.cachedData.groups;
        if (!cachedData || !cachedData.groups) {
            return null;
        }
        
        return {
            totalGroups: cachedData.total_groups || 0,
            activeGroups: cachedData.active_groups || 0,
            healthyEndpoints: cachedData.groups.reduce((sum, g) => sum + g.healthy_endpoints, 0),
            suspendedRequests: cachedData.total_suspended_requests || 0,
            activeGroup: cachedData.groups.find(group => group.is_active)
        };
    }
    
    // 检查是否有活跃组
    hasActiveGroup() {
        const overview = this.getGroupsOverview();
        return overview && overview.activeGroup;
    }
    
    // 获取活跃组信息
    getActiveGroup() {
        const overview = this.getGroupsOverview();
        return overview ? overview.activeGroup : null;
    }
    
    // 获取组列表
    getGroups() {
        const cachedData = this.webInterface.cachedData.groups;
        return cachedData ? cachedData.groups || [] : [];
    }
    
    // 根据名称获取组信息
    getGroupByName(groupName) {
        const groups = this.getGroups();
        return groups.find(group => group.name === groupName);
    }
    
    // 检查组是否可以激活
    canActivateGroup(groupName) {
        const group = this.getGroupByName(groupName);
        return group ? group.can_activate : false;
    }
    
    // 检查组是否可以暂停
    canPauseGroup(groupName) {
        const group = this.getGroupByName(groupName);
        return group ? group.can_pause : false;
    }
    
    // 检查组是否可以恢复
    canResumeGroup(groupName) {
        const group = this.getGroupByName(groupName);
        return group ? group.can_resume : false;
    }
    
    // 获取冷却中的组
    getCooldownGroups() {
        const groups = this.getGroups();
        return groups.filter(group => group.in_cooldown);
    }
    
    // 获取可用的组（不在冷却中且未暂停）
    getAvailableGroups() {
        const groups = this.getGroups();
        return groups.filter(group => !group.in_cooldown && group.can_activate);
    }
    
    // 格式化组状态文本
    formatGroupStatus(group) {
        if (group.in_cooldown) return '冷却中';
        if (group.is_active) return '活跃';
        if (group.status) return group.status;
        return '未激活';
    }
    
    // 获取组状态样式类
    getGroupStatusClass(group) {
        if (group.in_cooldown) return 'cooldown';
        if (group.is_active) return 'active';
        return 'inactive';
    }
    
    // 刷新组数据（强制重新加载）
    async refreshGroups() {
        try {
            await this.loadGroups();
        } catch (error) {
            console.error('刷新组数据失败:', error);
            Utils.showError('刷新组数据失败');
        }
    }
    
    // 批量操作：激活多个组
    async activateGroups(groupNames) {
        const results = [];
        for (const groupName of groupNames) {
            try {
                await this.activateGroup(groupName);
                results.push({ group: groupName, success: true });
            } catch (error) {
                results.push({ group: groupName, success: false, error: error.message });
            }
        }
        return results;
    }
    
    // 批量操作：暂停多个组
    async pauseGroups(groupNames) {
        const results = [];
        for (const groupName of groupNames) {
            try {
                await this.pauseGroup(groupName);
                results.push({ group: groupName, success: true });
            } catch (error) {
                results.push({ group: groupName, success: false, error: error.message });
            }
        }
        return results;
    }
    
    // 获取组健康度统计
    getGroupHealthStats() {
        const groups = this.getGroups();
        if (groups.length === 0) return null;

        const totalEndpoints = groups.reduce((sum, g) => sum + g.total_endpoints, 0);
        const healthyEndpoints = groups.reduce((sum, g) => sum + g.healthy_endpoints, 0);
        const unhealthyEndpoints = groups.reduce((sum, g) => sum + g.unhealthy_endpoints, 0);

        return {
            totalGroups: groups.length,
            totalEndpoints,
            healthyEndpoints,
            unhealthyEndpoints,
            healthPercentage: totalEndpoints > 0 ? (healthyEndpoints / totalEndpoints * 100).toFixed(1) : 0
        };
    }

    // 检查组是否支持应急激活
    canForceActivateGroup(groupName) {
        const group = this.getGroupByName(groupName);
        return group ? group.can_force_activate : false;
    }

    // 检查组是否处于应急激活状态
    isGroupForceActivated(groupName) {
        const group = this.getGroupByName(groupName);
        return group ? group.is_force_activated : false;
    }

    // 获取应急激活时间
    getForceActivationTime(groupName) {
        const group = this.getGroupByName(groupName);
        return group && group.is_force_activated ? group.force_activation_time : null;
    }

    // 获取应急激活的组列表
    getForceActivatedGroups() {
        const groups = this.getGroups();
        return groups.filter(group => group.is_force_activated);
    }
};

console.log('✅ GroupsManager模块已加载');
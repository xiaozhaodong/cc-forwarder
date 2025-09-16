// 组数据管理Hook - 基于实际useSSE实现
// 2025-09-16 按照迁移计划的完整技术规格实现

import { useState, useEffect, useCallback } from 'react';
import useSSE from '../../../hooks/useSSE.jsx';

const useGroupsData = () => {
  const [groups, setGroups] = useState(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [isInitialized, setIsInitialized] = useState(false);

  // SSE数据更新处理函数 - 使用实际的useSSE签名
  const handleSSEUpdate = useCallback((sseData, eventType) => {
    // 解包SSE数据 - 优先从data字段获取，否则使用sseData本身
    const payload = sseData.data || sseData;
    const { change_type: changeType } = payload;

    console.log(`📦 [组管理SSE] 收到${eventType || 'generic'}事件, 变更类型: ${changeType || 'none'}`, sseData);

    try {
      setGroups(prevGroups => {
        if (!prevGroups) return prevGroups;

        const newGroups = { ...prevGroups };

        // 1. 处理组状态变化事件 (eventType='group')
        if (eventType === 'group' || changeType === 'group_status_changed' || changeType === 'group_health_stats_changed') {
          console.log('👥 [组管理SSE] 处理组状态事件');

          // 1.1 处理完整组状态变化事件 (EventGroupStatusChanged)
          // 数据结构: { event, group, timestamp, details: { groups: [...], active_group, total_groups, ... } }
          if (payload.details && payload.details.groups) {
            console.log('📋 [组管理SSE] 处理完整组状态变化 - 从details.groups获取数据');

            // 合并details中的完整数据
            if (payload.details.groups) {
              newGroups.groups = payload.details.groups;
            }
            if (payload.details.active_group !== undefined) {
              newGroups.active_group = payload.details.active_group;
            }
            if (payload.details.total_groups !== undefined) {
              newGroups.total_groups = payload.details.total_groups;
            }
            if (payload.details.auto_switch_enabled !== undefined) {
              newGroups.auto_switch_enabled = payload.details.auto_switch_enabled;
            }
            if (payload.details.group_suspended_counts !== undefined) {
              newGroups.group_suspended_counts = payload.details.group_suspended_counts;
            }
            if (payload.details.total_suspended_requests !== undefined) {
              newGroups.total_suspended_requests = payload.details.total_suspended_requests;
            }

            // ⭐ 修正: 同步计算active_groups字段，确保与服务端一致
            if (newGroups.groups && Array.isArray(newGroups.groups)) {
              const activeCount = newGroups.groups.filter(g => g.is_active).length;
              newGroups.active_groups = activeCount;
              console.log(`🔢 [组管理SSE] 重新计算活跃组数量: ${activeCount}`);
            }

            console.log('✅ [组管理SSE] 完整组状态已更新 - 从details获取');
          }

          // 1.2 处理单组健康统计增量更新 (EventGroupHealthStatsChanged)
          // 数据结构: { group, healthy_endpoints, unhealthy_endpoints, total_endpoints, is_active, status, change_type, timestamp }
          else if (changeType === 'health_stats_changed' && payload.group) {
            console.log(`📊 [组管理SSE] 处理单组健康统计增量更新 - 组: ${payload.group}`);

            if (newGroups.groups && Array.isArray(newGroups.groups)) {
              // 查找并更新对应组的健康统计
              const groupIndex = newGroups.groups.findIndex(g => g.name === payload.group);
              if (groupIndex !== -1) {
                const updatedGroup = { ...newGroups.groups[groupIndex] };

                // 更新健康统计字段
                if (payload.healthy_endpoints !== undefined) {
                  updatedGroup.healthy_endpoints = payload.healthy_endpoints;
                }
                if (payload.unhealthy_endpoints !== undefined) {
                  updatedGroup.unhealthy_endpoints = payload.unhealthy_endpoints;
                }
                if (payload.total_endpoints !== undefined) {
                  updatedGroup.total_endpoints = payload.total_endpoints;
                }
                if (payload.is_active !== undefined) {
                  updatedGroup.is_active = payload.is_active;
                }
                if (payload.status !== undefined) {
                  updatedGroup.status = payload.status;
                  updatedGroup._computed_health_status = payload.status;
                }

                // ⭐ 修正: 仅在有完整字段信息时重新计算按钮状态，否则保持原值
                // 原因: EventGroupHealthStatsChanged 事件可能不包含 manually_paused, in_cooldown 等关键字段
                // 服务端计算逻辑: can_activate = healthyCount > 0 && !IsActive && !inCooldown
                //                can_pause = !ManuallyPaused
                //                can_resume = ManuallyPaused
                //                can_force_activate = healthyCount == 0 && !IsActive && !inCooldown

                if (payload.healthy_endpoints !== undefined &&
                    updatedGroup.in_cooldown !== undefined &&
                    updatedGroup.manually_paused !== undefined) {
                  // 只有在有完整信息时才重新计算，使用与服务端一致的逻辑
                  updatedGroup.can_activate = payload.healthy_endpoints > 0 &&
                                              !updatedGroup.is_active &&
                                              !updatedGroup.in_cooldown;
                  updatedGroup.can_pause = !updatedGroup.manually_paused;
                  updatedGroup.can_resume = updatedGroup.manually_paused;
                  updatedGroup.can_force_activate = payload.healthy_endpoints === 0 &&
                                                    !updatedGroup.is_active &&
                                                    !updatedGroup.in_cooldown;

                  console.log(`🔄 [组管理SSE] 重新计算按钮状态 - 组: ${payload.group}`);
                } else {
                  // 如果缺少关键字段，保持原有按钮状态不变，避免状态错误
                  console.log(`⚠️ [组管理SSE] 缺少关键字段，保持原有按钮状态 - 组: ${payload.group}`);
                }

                // 更新数组中的组数据
                newGroups.groups = [
                  ...newGroups.groups.slice(0, groupIndex),
                  updatedGroup,
                  ...newGroups.groups.slice(groupIndex + 1)
                ];

                // ⭐ 修正: 如果is_active状态发生变化，重新计算active_groups
                if (payload.is_active !== undefined &&
                    newGroups.groups[groupIndex].is_active !== updatedGroup.is_active) {
                  const activeCount = newGroups.groups.filter(g => g.is_active).length;
                  newGroups.active_groups = activeCount;
                  console.log(`🔢 [组管理SSE] 活跃状态变化，重新计算活跃组数量: ${activeCount}`);
                }

                console.log(`✅ [组管理SSE] 单组健康统计已更新 - 组: ${payload.group}, 健康端点: ${payload.healthy_endpoints}/${payload.total_endpoints}`);
              } else {
                console.warn(`⚠️ [组管理SSE] 未找到组 ${payload.group} 进行健康统计更新`);
              }
            }
          }

          // 1.3 处理直接组字段更新 (向后兼容)
          else if (payload.groups || payload.active_group !== undefined) {
            console.log('🔄 [组管理SSE] 处理直接组字段更新 - 向后兼容模式');

            if (payload.groups) {
              newGroups.groups = payload.groups;
            }
            if (payload.active_group !== undefined) {
              newGroups.active_group = payload.active_group;
            }
            if (payload.total_groups !== undefined) {
              newGroups.total_groups = payload.total_groups;
            }
            if (payload.auto_switch_enabled !== undefined) {
              newGroups.auto_switch_enabled = payload.auto_switch_enabled;
            }

            // ⭐ 修正: 在向后兼容模式中也计算active_groups
            if (newGroups.groups && Array.isArray(newGroups.groups)) {
              const activeCount = newGroups.groups.filter(g => g.is_active).length;
              newGroups.active_groups = activeCount;
              console.log(`🔢 [组管理SSE] 向后兼容模式 - 计算活跃组数量: ${activeCount}`);
            }

            console.log('✅ [组管理SSE] 直接组字段已更新');
          }
        }

        // 2. 处理连接统计更新 (eventType='connection') - 更新挂起请求数据
        if (eventType === 'connection' || changeType === 'connection_stats_updated') {
          console.log('🔗 [组管理SSE] 处理连接统计事件');

          // 更新挂起请求相关数据
          if (payload.group_suspended_counts !== undefined) {
            newGroups.group_suspended_counts = payload.group_suspended_counts;
          }
          if (payload.total_suspended_requests !== undefined) {
            newGroups.total_suspended_requests = payload.total_suspended_requests;
          }
          if (payload.max_suspended_requests !== undefined) {
            newGroups.max_suspended_requests = payload.max_suspended_requests;
          }

          console.log('✅ [组管理SSE] 挂起请求统计已更新');
        }

        // 更新时间戳
        newGroups.timestamp = new Date().toISOString();

        return newGroups;
      });
    } catch (error) {
      console.error('❌ [组管理SSE] 事件处理失败:', error, '事件数据:', sseData);
    }
  }, []);

  // 初始化SSE连接 - 使用实际的useSSE签名
  const { connectionStatus } = useSSE(handleSSEUpdate);

  const loadGroups = useCallback(async () => {
    try {
      // 只在首次加载时显示loading
      if (!isInitialized) {
        setLoading(true);
      }
      setError(null);

      const response = await fetch('/api/v1/groups');
      if (!response.ok) throw new Error('获取组信息失败');
      const data = await response.json();

      setGroups(data);
      setIsInitialized(true);
      setLoading(false);
    } catch (err) {
      setError(err.message);
      setLoading(false);
    }
  }, [isInitialized]);

  useEffect(() => {
    // 只在组件挂载时加载一次初始数据
    loadGroups();
  }, [loadGroups]);

  useEffect(() => {
    // 如果SSE连接失败，则使用定时刷新作为后备
    let interval = null;
    if (connectionStatus === 'failed' || connectionStatus === 'error') {
      console.log('🔄 [组管理React] SSE连接失败，启用定时刷新');
      interval = setInterval(loadGroups, 15000); // 15秒刷新一次
    }

    return () => {
      if (interval) {
        clearInterval(interval);
      }
    };
  }, [connectionStatus, loadGroups]);

  return {
    groups,
    loading,
    error,
    refreshGroups: loadGroups,
    isInitialized,
    sseConnectionStatus: connectionStatus
  };
};

export default useGroupsData;
export { useGroupsData };
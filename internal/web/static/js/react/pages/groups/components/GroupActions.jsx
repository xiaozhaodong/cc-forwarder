/**
 * 组操作按钮区组件（简化版本）
 *
 * 完全匹配原版简单按钮操作
 *
 * 创建日期: 2025-09-16
 * @author Claude Code Assistant
 */

/**
 * 组操作按钮区组件
 *
 * @param {Object} props - 组件属性
 * @param {Object} props.group - 组数据对象
 * @param {Function} props.onActivate - 激活组回调
 * @param {Function} props.onPause - 暂停组回调
 * @param {Function} props.onResume - 恢复组回调
 * @param {Function} props.onForceActivate - 强制激活组回调
 * @returns {JSX.Element} 操作按钮区JSX元素
 */
const GroupActions = ({
    group,
    onActivate = () => {},
    onPause = () => {},
    onResume = () => {},
    onForceActivate = () => {}
}) => {
    return (
        <div className="group-actions">
            {/* 激活按钮 - 始终渲染，通过disabled控制 */}
            <button
                className="group-btn btn-activate"
                onClick={() => onActivate()}
                disabled={!group.can_activate}
            >
                🚀 激活
            </button>

            {/* 应急激活按钮 - 只在can_force_activate为true时显示 */}
            {group.can_force_activate && (
                <button
                    className="group-btn btn-danger"
                    onClick={() => onForceActivate()}
                >
                    ⚡应急
                </button>
            )}

            {/* 暂停按钮 */}
            <button
                className="group-btn btn-pause"
                onClick={() => onPause()}
                disabled={!group.can_pause}
            >
                ⏸️ 暂停
            </button>

            {/* 恢复按钮 */}
            <button
                className="group-btn btn-resume"
                onClick={() => onResume()}
                disabled={!group.can_resume}
            >
                ▶️ 恢复
            </button>
        </div>
    );
};

export default GroupActions;

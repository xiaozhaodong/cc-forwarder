#!/bin/bash

# JavaScript错误修复脚本
# 创建时间: 2025-09-04 11:22:13

echo "🔧 修复Web界面JavaScript错误..."

# 创建一个简化版本的showTab函数，确保其始终可用
cat > temp_fix.js << 'EOF'
// 立即定义全局showTab函数，防止未定义错误
window.showTab = function(tabName) {
    console.log('📋 切换到标签页:', tabName);
    
    // 如果WebInterface已准备好，使用它
    if (window.webInterface && typeof window.webInterface.showTab === 'function') {
        window.webInterface.showTab(tabName);
        return;
    }
    
    // 否则提供基本的标签页切换功能
    try {
        // 隐藏所有标签页内容
        document.querySelectorAll('.tab-content').forEach(content => {
            content.classList.remove('active');
        });
        
        // 移除所有导航标签的活跃状态
        document.querySelectorAll('.nav-tab').forEach(tab => {
            tab.classList.remove('active');
        });
        
        // 显示目标标签页内容
        const targetContent = document.getElementById(tabName + '-content') || 
                             document.querySelector(`[data-tab="${tabName}"]`);
        if (targetContent) {
            targetContent.classList.add('active');
        }
        
        // 激活对应的导航标签
        const targetTab = document.querySelector(`[onclick*="${tabName}"]`);
        if (targetTab) {
            targetTab.classList.add('active');
        }
        
        console.log('✅ 基本标签切换完成:', tabName);
        
        // 等待WebInterface准备好后再尝试完整功能
        const retryCount = 0;
        const maxRetries = 50; // 5秒内重试
        const tryAgain = () => {
            if (window.webInterface && typeof window.webInterface.showTab === 'function') {
                console.log('🔄 WebInterface准备就绪，切换到完整功能');
                window.webInterface.showTab(tabName);
            } else if (retryCount < maxRetries) {
                retryCount++;
                setTimeout(tryAgain, 100);
            } else {
                console.warn('⚠️ WebInterface初始化超时，使用基本功能');
            }
        };
        setTimeout(tryAgain, 100);
        
    } catch (error) {
        console.error('❌ 标签切换错误:', error);
    }
};

// 确保函数在页面加载前就可用
console.log('✅ 全局showTab函数已定义');
EOF

# 将修复代码添加到app.js的开头
if [ -f "/Users/xiaozhaodong/JavaProjects/cc-forwarder/internal/web/static/js/app.js" ]; then
    # 创建备份
    cp "/Users/xiaozhaodong/JavaProjects/cc-forwarder/internal/web/static/js/app.js" "/Users/xiaozhaodong/JavaProjects/cc-forwarder/internal/web/static/js/app.js.bak"
    
    # 在文件开头添加修复代码
    cat temp_fix.js "/Users/xiaozhaodong/JavaProjects/cc-forwarder/internal/web/static/js/app.js.bak" > "/Users/xiaozhaodong/JavaProjects/cc-forwarder/internal/web/static/js/app.js"
    
    echo "✅ JavaScript修复已应用"
else
    echo "❌ 找不到app.js文件"
fi

# 清理临时文件
rm -f temp_fix.js

echo "🎉 修复完成！现在可以重启应用测试。"
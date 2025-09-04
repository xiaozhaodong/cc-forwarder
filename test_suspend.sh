#!/bin/bash

echo "🧪 测试请求挂起功能"
echo "=================="

echo "1. 启动应用程序 (后台运行)"
./cc-forwarder -config config/config.yaml --no-tui &
APP_PID=$!

echo "应用程序启动，PID: $APP_PID"
echo "等待应用启动完成..."
sleep 3

echo ""
echo "2. 发送测试请求 (这应该会触发挂起)"
echo "请求将会被挂起，等待手动组切换..."

curl -H "Authorization: Bearer sk-UUqlTlGquGjs3zlKXKIXsGQf5XQeXoxiRqKGUZvjb3yq8U0e" \
     -H "Content-Type: application/json" \
     -d '{
       "model": "claude-3-sonnet-20240229",
       "max_tokens": 10,
       "messages": [{"role": "user", "content": "Hello"}]
     }' \
     http://localhost:8088/v1/messages &

CURL_PID=$!
echo "请求发送中，PID: $CURL_PID"

echo ""
echo "3. 等待10秒观察挂起效果..."
sleep 10

echo ""
echo "4. 检查应用日志中的挂起信息..."
echo "看看是否有挂起相关的日志输出"

echo ""
echo "5. 清理进程"
kill $CURL_PID 2>/dev/null
kill $APP_PID 2>/dev/null
wait $APP_PID 2>/dev/null

echo "测试完成"
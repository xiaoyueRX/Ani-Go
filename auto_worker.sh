#!/bin/bash

# Remove old logs to start fresh
rm -f worker_output.log

echo "Starting initial work..."
hermes chat --yolo -m "deepseek-v4-flash" --provider deepseek -q "You are assigned to work on /root/x/Ani-Go while the user sleeps. Refactor the Web UI to be beautiful and modern (Vue3+Tailwind+DaisyUI). Fix backend bugs. Work autonomously. Take your time, test thoroughly. This is an overnight task." > worker_output.log 2>&1

while true; do
    echo "Agent stopped. Restarting with --continue in 10 seconds..."
    sleep 10
    hermes chat --yolo --continue -q "Your previous run was interrupted. Please review your progress and continue the refactoring and UI modernization work where you left off. Do not stop until the web UI is perfect." >> worker_output.log 2>&1
done
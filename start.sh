#!/usr/bin/env bash
# Запуск Go-бэкенда конвертера в фоне (переживает выход из SSH).
# Слушает только localhost, наружу отдаёт reverse-proxy сервера.
# Порт по умолчанию 8137. Изменить: PORT=9000 bash start.sh
cd "$(dirname "$0")"

PORT="${PORT:-8137}"
mkdir -p logs

# Останавливаем прошлый процесс (Go-бинарник или старый Python/uvicorn)
if [ -f app.pid ] && kill -0 "$(cat app.pid)" 2>/dev/null; then
    echo "Останавливаю прошлый процесс ($(cat app.pid))..."
    kill "$(cat app.pid)" 2>/dev/null || true
    sleep 1
fi
pkill -f "uvicorn backend.main" 2>/dev/null || true

if [ ! -f .env ]; then
    echo "⚠️  Нет файла .env с ключом! Сначала: echo 'GEMINI_API_KEY=КЛЮЧ' > .env"
    exit 1
fi

chmod +x ./converter-go 2>/dev/null || true
PORT="$PORT" nohup ./converter-go > logs/app.log 2>&1 &
echo $! > app.pid
sleep 1
echo "🚀 Go-бэкенд запущен на 127.0.0.1:$PORT (PID $(cat app.pid))"
echo "   Логи:    tail -f ~/converter/logs/app.log"
echo "   Проверка: curl -s http://127.0.0.1:$PORT/api/health"

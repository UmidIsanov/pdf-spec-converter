#!/usr/bin/env bash
# Запуск сервиса конвертера в фоне (переживает выход из SSH).
# Порт по умолчанию 8137, слушает только localhost (наружу отдаёт reverse-proxy сервера).
# Изменить порт: PORT=9000 bash start.sh
cd "$(dirname "$0")"

PORT="${PORT:-8137}"
mkdir -p logs

# Останавливаем прошлый экземпляр, если запущен
if [ -f app.pid ] && kill -0 "$(cat app.pid)" 2>/dev/null; then
    echo "Останавливаю прошлый процесс ($(cat app.pid))..."
    kill "$(cat app.pid)" 2>/dev/null || true
    sleep 1
fi

if [ ! -f .env ]; then
    echo "⚠️  Нет файла .env с ключом! Сначала: echo 'GEMINI_API_KEY=КЛЮЧ' > .env"
    exit 1
fi

nohup ./venv/bin/python -m uvicorn backend.main:app --host 127.0.0.1 --port "$PORT" > logs/app.log 2>&1 &
echo $! > app.pid
sleep 2
echo "🚀 Запущен на 127.0.0.1:$PORT (PID $(cat app.pid))"
echo "   Логи:    tail -f ~/converter/logs/app.log"
echo "   Проверка: curl -s http://127.0.0.1:$PORT/api/health"
